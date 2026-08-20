package builder

import (
	"html/template"
	"os"
	"path/filepath"
)

// rootIndexTmpl is the landing page at /. It links each service to its
// protected branches first, then its recent ones.
var rootIndexTmpl = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Mojro Docs</title>
<style>
  :root { color-scheme: light dark; }
  body {
    margin: 0; padding: 3rem 1.5rem;
    font: 16px/1.6 system-ui, -apple-system, "Segoe UI", sans-serif;
    background: #fafafa; color: #18181b;
  }
  main { max-width: 52rem; margin: 0 auto; }
  h1 { font-size: 1.75rem; margin: 0 0 .25rem; }
  .sub { color: #71717a; margin: 0 0 2.5rem; }
  .svc {
    background: #fff; border: 1px solid #e4e4e7; border-radius: .5rem;
    padding: 1rem 1.25rem; margin-bottom: .75rem;
  }
  .svc h2 { font-size: 1.05rem; margin: 0 0 .6rem; }
  .svc h2 a { color: #4f46e5; text-decoration: none; }
  .svc h2 a:hover { text-decoration: underline; }
  .row { display: flex; flex-wrap: wrap; gap: .35rem; align-items: center; }
  .row + .row { margin-top: .4rem; }
  .lbl {
    font-size: .65rem; text-transform: uppercase; letter-spacing: .04em;
    color: #a1a1aa; margin-right: .25rem; min-width: 4.5rem;
  }
  a.v {
    font-size: .8rem; text-decoration: none; padding: .15rem .45rem;
    border-radius: .25rem; background: #f4f4f5; color: #3f3f46;
    border: 1px solid #e4e4e7; word-break: break-all;
  }
  a.v:hover { background: #e4e4e7; }
  a.v.prot { background: #eef2ff; border-color: #c7d2fe; color: #4338ca; font-weight: 600; }
  .empty { color: #71717a; }
  @media (prefers-color-scheme: dark) {
    body { background: #09090b; color: #f4f4f5; }
    .svc { background: #18181b; border-color: #27272a; }
    a.v { background: #27272a; border-color: #3f3f46; color: #d4d4d8; }
    a.v:hover { background: #3f3f46; }
    a.v.prot { background: #312e81; border-color: #4338ca; color: #e0e7ff; }
    .svc h2 a { color: #a5b4fc; }
  }
</style>
</head>
<body>
<main>
  <h1>Mojro Docs</h1>
  <p class="sub">Documentation for all Mojro API services, published from each repo's <code>docs/</code> folder.</p>
  {{- if not .Services }}
  <p class="empty">No documentation published yet.</p>
  {{- end }}
  {{- range .Services }}
  <div class="svc">
    <h2><a href="/{{ .Service }}/{{ .Default }}/">{{ .Service }}</a></h2>
    {{- if .Protected }}
    <div class="row">
      <span class="lbl">Branches</span>
      {{- range .Protected }}
      <a class="v prot" href="/{{ .Service }}/{{ .Name }}/">{{ .Name }}</a>
      {{- end }}
    </div>
    {{- end }}
    {{- if .Recent }}
    <div class="row">
      <span class="lbl">Recent</span>
      {{- range .Recent }}
      <a class="v" href="/{{ .Service }}/{{ .Name }}/">{{ .Name }}</a>
      {{- end }}
    </div>
    {{- end }}
  </div>
  {{- end }}
</main>
</body>
</html>
`))

type indexVersion struct {
	Service string
	Name    string
}

type indexService struct {
	Service   string
	Default   string
	Protected []indexVersion
	Recent    []indexVersion
}

type indexData struct {
	Services []indexService
}

// writeRootIndex regenerates the landing page from the version manifests.
func (b *Builder) writeRootIndex() error {
	var data indexData
	for _, vl := range b.Services() {
		svc := indexService{Service: vl.Service}
		for _, v := range vl.Versions {
			iv := indexVersion{Service: vl.Service, Name: v.Name}
			if v.Protected {
				svc.Protected = append(svc.Protected, iv)
			} else {
				svc.Recent = append(svc.Recent, iv)
			}
		}
		// Prefer a protected branch as the service's default landing target.
		if len(svc.Protected) > 0 {
			svc.Default = svc.Protected[0].Name
		} else if len(svc.Recent) > 0 {
			svc.Default = svc.Recent[0].Name
		} else {
			continue
		}
		data.Services = append(data.Services, svc)
	}

	if err := os.MkdirAll(b.cfg.SiteDir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(b.cfg.SiteDir, "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()
	return rootIndexTmpl.Execute(f, data)
}
