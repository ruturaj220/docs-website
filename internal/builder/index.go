package builder

import (
	"html/template"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mojro/docs-platform/internal/assets"
)

var (
	rootIndexTmpl = template.Must(template.New("index").Parse(string(assets.IndexHTML)))
	serviceTmpl   = template.Must(template.New("service").Parse(string(assets.ServiceHTML)))
)

type indexVersion struct {
	Service     string
	DisplayName string
	Name        string
	Href        string
	When        string
}

type indexService struct {
	Service      string
	DisplayName  string
	Description  string
	SeeAllHref   string
	DefaultHref  string
	DefaultName  string
	VersionLabel string
	Protected    []indexVersion
	Recent       []indexVersion
}

type indexData struct {
	Year int
	// Services is every published service, one card each. Regenerated after
	// every build, so a newly pushed service appears without a restart.
	Services []indexService
	// Primary is the first published service, for the incidental links that
	// sit outside the service grid.
	Primary *indexService
}

type servicePageData struct {
	Year    int
	Service indexService
}

// WriteRootIndex regenerates the home page, service overview pages, and assets.
func (b *Builder) WriteRootIndex() error {
	return b.writeRootIndex()
}

func (b *Builder) writeRootIndex() error {
	year := time.Now().UTC().Year()
	services := collectIndexServices(b.Services())

	if err := os.MkdirAll(b.cfg.SiteDir, 0o755); err != nil {
		return err
	}
	if err := writeRootAssets(b.cfg.SiteDir); err != nil {
		return err
	}

	home := indexData{Year: year, Services: services}
	if len(services) > 0 {
		home.Primary = &services[0]
	}
	if err := writeTemplate(filepath.Join(b.cfg.SiteDir, "index.html"), rootIndexTmpl, home); err != nil {
		return err
	}

	// Real data for every published service at /<service>/ (See all target).
	for _, svc := range services {
		dir := filepath.Join(b.cfg.SiteDir, svc.Service)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := writeTemplate(filepath.Join(dir, "index.html"), serviceTmpl, servicePageData{
			Year:    year,
			Service: svc,
		}); err != nil {
			return err
		}
	}
	return nil
}

func collectIndexServices(lists []VersionList) []indexService {
	out := make([]indexService, 0, len(lists))
	for _, vl := range lists {
		display := displayName(vl.Service)
		svc := indexService{
			Service:     vl.Service,
			DisplayName: display,
			Description: "Technical documentation for " + display + ", published from this service's docs folder.",
			SeeAllHref:  "/" + vl.Service + "/",
		}
		for _, v := range vl.Versions {
			iv := indexVersion{
				Service:     vl.Service,
				DisplayName: display,
				Name:        v.Name,
				Href:        "/" + vl.Service + "/" + v.Name + "/",
				When:        v.BuiltAt.UTC().Format("2 Jan 2006"),
			}
			if v.Protected {
				svc.Protected = append(svc.Protected, iv)
			} else {
				svc.Recent = append(svc.Recent, iv)
			}
		}
		if n := len(svc.Protected) + len(svc.Recent); n == 1 {
			svc.VersionLabel = "1 version"
		} else {
			svc.VersionLabel = strconv.Itoa(n) + " versions"
		}

		if len(svc.Protected) > 0 {
			svc.DefaultHref = svc.Protected[0].Href
			svc.DefaultName = svc.Protected[0].Name
		} else if len(svc.Recent) > 0 {
			svc.DefaultHref = svc.Recent[0].Href
			svc.DefaultName = svc.Recent[0].Name
		} else {
			continue
		}
		out = append(out, svc)
	}
	return out
}

func writeTemplate(path string, tmpl *template.Template, data any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, data)
}

func writeRootAssets(siteDir string) error {
	files := map[string][]byte{
		"assets/home/index.css":         assets.IndexCSS,
		"assets/home/index.js":          assets.IndexJS,
		"assets/home/service.css":       assets.ServiceCSS,
		"assets/img/mojro-logo.png":     assets.Logo,
		"assets/img/mojro-wordmark.png": assets.Wordmark,
	}
	for dest, body := range files {
		full := filepath.Join(siteDir, dest)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func displayName(service string) string {
	parts := strings.Split(service, "-")
	for i, p := range parts {
		switch strings.ToLower(p) {
		case "api":
			parts[i] = "API"
		case "sdk":
			parts[i] = "SDK"
		default:
			if p == "" {
				continue
			}
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
