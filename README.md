# docs-platform

One documentation site for all Mojro API services, at `docs.mojro.com`.

Every service repo keeps a `docs/` folder containing only `.md` files. This
service pulls those folders in on push, assembles them into a single MkDocs
tree, renders it, and serves the result directly — no nginx sidecar, no
per-service mkdocs config.

## Architecture

```
Bitbucket: 20+ API repos, each with a docs/ dir of .md files
        │
        │  POST /webhook/<service>?token=…&clone_url=…
        ▼
┌──────────────────────────────────────────────────────┐
│ docs-platform (Go)                                   │
│  • fetches the pushed branch of that service         │
│  • generates a per-service mkdocs.yml (INHERITs the  │
│    shared theme) so repos hold only .md files        │
│  • runs `mkdocs build` for that (service, branch)    │
│  • updates versions.json + applies retention         │
│  • serves every version directly (net/http)          │
└──────────────────────────────────────────────────────┘
        │ PVC (Azure Files) — repos, build trees, sites
        ▼
   Envoy Gateway  ──►  https://docs.mojro.com
```

### Versioned layout

Each service is versioned independently by branch, the way Read the Docs
models a project — with a version flyout in the corner of every page for
switching between them:

```
/                                     landing page: services + versions
/shipper-api/versions.json            version list (drives the flyout)
/shipper-api/main/                    main branch
/shipper-api/develop/                 develop branch
/shipper-api/feature/MJ-8581-x/       feature branch
/auth-api/main/                       …one tree per service
```

### What a service repo contains

Only Markdown, and any images it references:

```
<service>/docs/
├── redis-entraid-auth-flow.md
├── metadata-developer-guide.md
└── assets/redis-auth.png
```

No `mkdocs.yml`, no `requirements.txt`, no theme files, and **no `index.md`** —
maintaining one per repo across 20+ services is exactly the toil this avoids.
The landing page for every service is rendered from this repo's shared
`docs/index.md`, which is a Go template with:

| Field | Value |
|---|---|
| `{{ .Service }}` | service name, e.g. `mojro-common` |
| `{{ .Branch }}` | branch being built |
| `{{ .Pages }}` | that service's pages, each with `.Title` and `.File` |
| `{{ .RepoHint }}` | link to the service's Bitbucket repo |

Page titles come from each file's first `# heading`, falling back to a tidied
filename. A service that genuinely wants a custom landing page can add its own
`docs/index.md` and it takes precedence.

### Branch policy

- **Protected branches** (`PROTECTED_REFS`, default `main,master,develop`)
  are always published, update in place, and are never evicted.
- **Feature branches** are published as their own versions while
  `PUBLISH_FEATURE_BRANCHES=true`, and the newest `MAX_VERSIONS` (default 5)
  are retained per service. Older ones are deleted from disk as new ones land.
- To stop publishing feature branches later, set
  `webhook.publishFeatureBranches=false` in the chart. Their pushes are then
  acknowledged with 200 and skipped — no Bitbucket retries.

## Why Go rather than Python/Flask

The service is Go, but the runtime image still ships Python because **mkdocs
is a Python tool with no Go equivalent** — Go shells out to it. What Go buys
here is that `net/http.FileServer` serves the rendered site natively, which
is what let the nginx sidecar be removed entirely, plus goroutine-based build
queueing and a single static binary.

## Layout

```
cmd/server/          entrypoint, config from env, graceful shutdown
internal/builder/    git fetch, per-version build, retention, landing page
internal/handler/    webhook endpoints + static file serving
internal/assets/     logo + version-flyout JS/CSS, embedded into the binary
mkdocs-base.yml      shared theme/plugins; each build INHERITs this
docs/index.md        shared landing page rendered for every service
helm/docs-platform/  chart: Deployment, Service, PVC, Secret, HTTPRoute
Dockerfile           multi-stage: Go build -> python+mkdocs runtime
```

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `WEBHOOK_SECRET` | *(required)* | Shared token webhooks must present |
| `DATA_DIR` | `/data` | Holds `repos/`, `tree/`, `site/` |
| `LISTEN_ADDR` | `:8080` | Listen address |
| `MKDOCS_BASE_CONFIG` | `/app/mkdocs-base.yml` | Shared theme config each build inherits |
| `COMMON_INDEX` | `/app/docs/index.md` | Shared landing page template for services |
| `REPO_WORKSPACE` | `mojro` | Bitbucket workspace, for repo links in that template |
| `PROTECTED_REFS` | `main,master,develop` | Long-lived branches: always published, never evicted |
| `PUBLISH_FEATURE_BRANCHES` | `true` | Publish non-protected branches as versions |
| `MAX_VERSIONS` | `5` | Non-protected versions kept per service (newest first) |

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| POST | `/webhook/{service}` | Build that service's pushed branch as a version |
| GET | `/healthz` | Liveness |
| GET | `/readyz` | Ready only once a site has been built |
| GET | `/api/services` | JSON: every service and its versions |
| GET | `/` | Landing page listing services and versions |
| GET | `/{service}/versions.json` | Version list the flyout reads |
| GET | `/{service}/{branch}/**` | That version's rendered site |

Service names must be lowercase alphanumeric with dashes (`auth-api`);
anything else is rejected with 400 rather than sanitized, since the name
becomes both a directory and a public URL segment.

## Local development

Needs Go and `mkdocs` on `PATH` (`pip install -r requirements.txt`).

```bash
pip install -r requirements.txt
WEBHOOK_SECRET=dev-secret ./scripts/run-local.sh   # listens on :8080
```

`run-local.sh` publishes feature branches by default, so you can demo without
write access to `main`.

Trigger a build by hand:

```bash
curl -X POST "http://localhost:8080/webhook/shipper-api\
?token=dev-secret\
&clone_url=https://<user>:<app-password>@bitbucket.org/mojro/shipper-api.git\
&ref=main"

open http://localhost:8080/
```

### Testing real Bitbucket webhooks locally

Bitbucket needs a public URL to reach your laptop, so front it with ngrok:

```bash
ngrok config add-authtoken <your-token>    # one time
ngrok http 8080
```

Then add a webhook on the service repo (Repo settings -> Webhooks, trigger
on Push) pointing at the forwarding URL:

```
https://<id>.ngrok-free.app/webhook/shipper-api?token=dev-secret&clone_url=https://<user>:<app-password>@bitbucket.org/mojro/shipper-api.git
```

Omit `ref` — the pushed branch is read from the Bitbucket payload.

## Deploying

```bash
make image TAG=0.1.0
make push  TAG=0.1.0

# Create the webhook secret out of band, then reference it:
kubectl -n docs-platform create secret generic docs-platform-webhook \
  --from-literal=webhook-secret="$(openssl rand -hex 32)"

helm upgrade --install docs helm/docs-platform \
  --namespace docs-platform --create-namespace \
  --set webhook.existingSecret=docs-platform-webhook \
  --set gateway.hostname=docs.mojro.com
```

The chart assumes an Envoy Gateway named `envoy-gateway` in
`envoy-gateway-system` already exists and emits an `HTTPRoute` attached to
it. Set `gateway.enabled=false` to skip that and expose the Service another
way.

### Storage

`persistence.storageClass` defaults to `azurefile-csi` with
`ReadWriteMany`. Azure Files (rather than Azure Disk) keeps the option of
running more than one replica later; for a single replica, `managed-csi`
with `ReadWriteOnce` also works.

## Wiring Bitbucket

Per service repo: Repo settings → Webhooks → add, trigger on Push:

```
https://docs.mojro.com/webhook/<service>?token=<secret>&clone_url=https://x-token-auth:<app-password>@bitbucket.org/<workspace>/<service>.git
```

The pushed branch is read from the Bitbucket payload, so `ref` can be
omitted. Since services build to `docs/<service>/`, a push to any one repo
rebuilds the whole site; concurrent pushes collapse into a single rebuild
rather than queueing one per push.

## Known limitations

- **Single replica.** Builds write to the shared volume; with RWX storage
  multiple replicas can serve reads, but builds should stay on one pod.
- **No build history or UI.** Results are in pod logs only.
- **Token in query string.** Fine behind the Gateway with TLS, but
  Bitbucket's HMAC signature would be stronger than a shared token.
- **Credentials in `clone_url`.** Each webhook carries an app password in its
  URL. Acceptable for an internal tool, but per-service credentials held in a
  Secret would be better before this grows.
- **A service is only removed from the site manually** — deleting its
  `docs/` folder stops updates but leaves the last build in the tree.
- **Branch deletion doesn't unpublish.** A deleted feature branch keeps its
  version until retention evicts it.
- **Nav is per service.** Each version is its own MkDocs site, so there is no
  cross-service search; the landing page is the only shared index.
