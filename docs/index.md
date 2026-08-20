# {{ .Service }}

Documentation for `{{ .Service }}`, built from branch `{{ .Branch }}`.
{{ if .Pages }}
## Pages

{{ range .Pages }}- [{{ .Title }}]({{ .File }})
{{ end }}{{ end }}
## About these docs

These pages are published automatically from the `docs/` folder of the
[{{ .Service }}]({{ .RepoHint }}) repository. To change them, edit the Markdown
in that repo and push — this site rebuilds itself.

Only `.md` files (and any images they reference) belong in a service's `docs/`
folder. Theme, navigation, and this landing page are maintained centrally in
the docs-platform repository, so no service needs its own MkDocs config.

!!! tip "Overriding this page"
    A service that wants a custom landing page can add its own
    `docs/index.md` — it takes precedence over this shared one.
