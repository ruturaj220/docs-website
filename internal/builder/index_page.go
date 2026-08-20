package builder

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

// page is one Markdown file in a service's docs/ folder.
type page struct {
	// File is the filename as linked from the index (e.g. "setup.md").
	File string
	// Title is the page's first H1 if it has one, else a tidied filename.
	Title string
}

// indexPageData is what the common index.md template is rendered with.
type indexPageData struct {
	Service  string
	Branch   string
	Pages    []page
	RepoHint string
}

// writeIndexPage makes sure the build has a docs/index.md.
//
// Services are expected to write only their own content pages, so the landing
// page comes from the platform's shared template (CommonIndex). A service that
// does want its own landing page can add docs/index.md and that wins.
func (b *Builder) writeIndexPage(docsDir, service, branch string) error {
	for _, name := range []string{"index.md", "README.md"} {
		if _, err := os.Stat(filepath.Join(docsDir, name)); err == nil {
			return nil
		}
	}

	data := indexPageData{
		Service:  service,
		Branch:   branch,
		Pages:    collectPages(docsDir),
		RepoHint: fmt.Sprintf("https://bitbucket.org/%s/%s", b.cfg.RepoWorkspace, service),
	}

	body, err := b.renderCommonIndex(data)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(docsDir, "index.md"), []byte(body), 0o644)
}

func (b *Builder) renderCommonIndex(data indexPageData) (string, error) {
	raw, err := os.ReadFile(b.cfg.CommonIndex)
	if err != nil {
		// Without the shared template there's still no index.md, and MkDocs
		// would leave the version root empty — emit a minimal page instead.
		return minimalIndex(data), nil
	}

	tmpl, err := template.New("index").Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", b.cfg.CommonIndex, err)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("render %s: %w", b.cfg.CommonIndex, err)
	}
	return sb.String(), nil
}

func minimalIndex(data indexPageData) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", data.Service)
	fmt.Fprintf(&sb, "Documentation for `%s`, built from branch `%s`.\n\n",
		data.Service, data.Branch)
	if len(data.Pages) > 0 {
		sb.WriteString("## Pages\n\n")
		for _, p := range data.Pages {
			fmt.Fprintf(&sb, "- [%s](%s)\n", p.Title, p.File)
		}
	}
	return sb.String()
}

// collectPages lists the top-level Markdown pages, titled by their first H1.
func collectPages(docsDir string) []page {
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		return nil
	}

	var pages []page
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		if strings.EqualFold(e.Name(), "index.md") {
			continue
		}
		pages = append(pages, page{
			File:  e.Name(),
			Title: pageTitle(filepath.Join(docsDir, e.Name())),
		})
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Title < pages[j].Title })
	return pages
}

// pageTitle returns a page's first H1, falling back to a tidied filename.
func pageTitle(path string) string {
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "# ") {
				if t := strings.TrimSpace(strings.TrimPrefix(line, "# ")); t != "" {
					return t
				}
			}
		}
	}

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
	return name
}
