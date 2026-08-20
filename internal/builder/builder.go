// Package builder checks out service repos and renders one versioned MkDocs
// site per (service, branch).
package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/mojro/docs-platform/internal/assets"
)

// Config describes where the builder keeps its working state.
type Config struct {
	// ReposDir holds one git checkout per service repo.
	ReposDir string
	// TreeDir holds the assembled per-build docs source.
	TreeDir string
	// SiteDir receives rendered HTML at <service>/<branch>/.
	SiteDir string
	// MetaDir holds the per-service version manifests.
	MetaDir string
	// BaseConfig is the shared mkdocs config each build INHERITs.
	BaseConfig string
	// CommonIndex is the shared index.md template used as a service's landing
	// page when the service repo doesn't provide its own.
	CommonIndex string
	// RepoWorkspace is the Bitbucket workspace, used to build repo links in the
	// shared index template.
	RepoWorkspace string
	// ProtectedRefs are long-lived branches: always publishable, never evicted.
	ProtectedRefs []string
	// MaxVersions caps how many non-protected versions are retained.
	MaxVersions int
}

// Builder serializes builds so concurrent runs can't interleave writes to the
// same output tree.
type Builder struct {
	cfg Config
	mu  sync.Mutex

	// Parsed MkDocs search indexes, one shard per published version, reloaded
	// when a build rewrites the file. Guarded by searchMu.
	searchMu     sync.Mutex
	searchShards map[string]*searchShard
}

func New(cfg Config) *Builder {
	return &Builder{cfg: cfg}
}

var serviceNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ValidServiceName reports whether a webhook-supplied service name is safe to
// use as a directory name and URL segment.
func ValidServiceName(s string) bool {
	return len(s) <= 63 && serviceNamePattern.MatchString(s)
}

var branchSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidBranchName allows the "/" that real branches use (feature/MJ-123) while
// rejecting traversal, absolute paths, and git refspec oddities.
func ValidBranchName(s string) bool {
	if s == "" || len(s) > 200 || strings.Contains(s, "..") {
		return false
	}
	segs := strings.Split(s, "/")
	if len(segs) > 4 {
		return false
	}
	for _, seg := range segs {
		if !branchSegmentPattern.MatchString(seg) {
			return false
		}
	}
	return true
}

// IsProtected reports whether a branch is one of the long-lived branches.
func (b *Builder) IsProtected(branch string) bool {
	for _, p := range b.cfg.ProtectedRefs {
		if p == branch {
			return true
		}
	}
	return false
}

// Build checks out service@branch and renders it to site/<service>/<branch>/,
// then updates the version manifest and prunes evicted versions.
func (b *Builder) Build(service, cloneURL, branch string) error {
	if !ValidServiceName(service) {
		return fmt.Errorf("invalid service name %q", service)
	}
	if !ValidBranchName(branch) {
		return fmt.Errorf("invalid branch name %q", branch)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	repoPath := filepath.Join(b.cfg.ReposDir, service)
	if err := os.MkdirAll(b.cfg.ReposDir, 0o755); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
		if err := run("", "git", "clone", "--no-checkout", cloneURL, repoPath); err != nil {
			return fmt.Errorf("clone %s: %w", service, err)
		}
	}

	// Fetch the exact ref so branches with "/" work and only what's needed
	// comes down.
	if err := run(repoPath, "git", "fetch", "--depth", "1", "origin",
		"+refs/heads/"+branch+":refs/remotes/origin/"+branch); err != nil {
		return fmt.Errorf("fetch %s@%s: %w", service, branch, err)
	}
	if err := run(repoPath, "git", "checkout", "--force", "-B", branch,
		"refs/remotes/origin/"+branch); err != nil {
		return fmt.Errorf("checkout %s@%s: %w", service, branch, err)
	}

	srcDocs := filepath.Join(repoPath, "docs")
	if _, err := os.Stat(srcDocs); os.IsNotExist(err) {
		return fmt.Errorf("service %s@%s has no docs/ directory", service, branch)
	}

	// Assemble an isolated source tree for this build.
	work := filepath.Join(b.cfg.TreeDir, service, branch)
	if err := os.RemoveAll(work); err != nil {
		return err
	}
	workDocs := filepath.Join(work, "docs")
	if err := os.MkdirAll(workDocs, 0o755); err != nil {
		return err
	}
	if err := copyTree(srcDocs, workDocs); err != nil {
		return fmt.Errorf("copy docs: %w", err)
	}
	if err := writeSelectorAssets(workDocs); err != nil {
		return fmt.Errorf("write selector assets: %w", err)
	}
	if err := b.writeIndexPage(workDocs, service, branch); err != nil {
		return fmt.Errorf("write index page: %w", err)
	}
	if err := b.writeServiceConfig(work, service, branch); err != nil {
		return fmt.Errorf("write mkdocs config: %w", err)
	}

	outDir := filepath.Join(b.cfg.SiteDir, service, branch)
	if err := run(work, "mkdocs", "build", "--clean", "--site-dir", outDir); err != nil {
		return fmt.Errorf("mkdocs build %s@%s: %w", service, branch, err)
	}

	evicted, err := b.recordVersion(service, branch, b.IsProtected(branch))
	if err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	for _, v := range evicted {
		_ = os.RemoveAll(filepath.Join(b.cfg.SiteDir, service, v.Name))
		_ = os.RemoveAll(filepath.Join(b.cfg.TreeDir, service, v.Name))
	}

	return b.writeRootIndex()
}

// writeServiceConfig emits a minimal per-service mkdocs.yml that inherits the
// shared theme, so service repos never carry mkdocs config of their own.
func (b *Builder) writeServiceConfig(work, service, branch string) error {
	cfg := fmt.Sprintf(`INHERIT: %s
site_name: %s
docs_dir: docs
extra:
  service: %s
  branch: %s
`, b.cfg.BaseConfig, service, service, branch)
	return os.WriteFile(filepath.Join(work, "mkdocs.yml"), []byte(cfg), 0o644)
}

func writeSelectorAssets(docsDir string) error {
	files := map[string][]byte{
		"assets/js/version-selector.js":   assets.VersionSelectorJS,
		"assets/css/version-selector.css": assets.VersionSelectorCSS,
		"assets/css/theme.css":            assets.ThemeCSS,
		"assets/img/mojro-logo.png":       assets.Logo,
	}
	for dest, data := range files {
		full := filepath.Join(docsDir, dest)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func run(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyTree(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}
