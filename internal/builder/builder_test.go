package builder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidServiceName(t *testing.T) {
	valid := []string{"auth-api", "shipper-api", "a", "geo-resolver-api", "api2"}
	for _, s := range valid {
		if !ValidServiceName(s) {
			t.Errorf("ValidServiceName(%q) = false, want true", s)
		}
	}

	invalid := []string{
		"", "..", "../../etc", "....etc", "-lead", "trail-",
		"Upper-API", "has space", "has/slash", "has_underscore", "dot.name",
	}
	for _, s := range invalid {
		if ValidServiceName(s) {
			t.Errorf("ValidServiceName(%q) = true, want false", s)
		}
	}
}

func TestValidBranchName(t *testing.T) {
	valid := []string{
		"main", "master", "develop",
		"feature/MJ-8581-azure-managed-redis-entraid-auth",
		"release/1.2.3", "bump/argo-cd-v3.5.0", "hotfix/a/b",
	}
	for _, s := range valid {
		if !ValidBranchName(s) {
			t.Errorf("ValidBranchName(%q) = false, want true", s)
		}
	}

	invalid := []string{
		"", "..", "../etc", "feature/../../etc", "/abs", "trailing/",
		"a//b", "-lead", "has space", "a/b/c/d/e", "feature/-x",
	}
	for _, s := range invalid {
		if ValidBranchName(s) {
			t.Errorf("ValidBranchName(%q) = true, want false", s)
		}
	}
}

func TestIsProtected(t *testing.T) {
	b := New(Config{ProtectedRefs: []string{"main", "master", "develop"}})
	for _, s := range []string{"main", "master", "develop"} {
		if !b.IsProtected(s) {
			t.Errorf("IsProtected(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"feature/x", "release/1.0", ""} {
		if b.IsProtected(s) {
			t.Errorf("IsProtected(%q) = true, want false", s)
		}
	}
}

func TestApplyRetentionKeepsProtectedAndNewestN(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(name string, prot bool, ageMin int) Version {
		return Version{Name: name, Protected: prot, BuiltAt: base.Add(time.Duration(ageMin) * time.Minute)}
	}

	versions := []Version{
		mk("main", true, 0),
		mk("develop", true, 1),
		mk("feature/a", false, 10), // oldest feature
		mk("feature/b", false, 20),
		mk("feature/c", false, 30),
		mk("feature/d", false, 40),
		mk("feature/e", false, 50),
		mk("feature/f", false, 60), // newest feature
	}

	kept, evicted := applyRetention(versions, 5)

	if len(evicted) != 1 || evicted[0].Name != "feature/a" {
		t.Fatalf("evicted = %v, want only feature/a", names(evicted))
	}

	// Both protected branches survive plus the 5 newest feature branches.
	if len(kept) != 7 {
		t.Fatalf("kept %d versions (%v), want 7", len(kept), names(kept))
	}
	for _, want := range []string{"main", "develop", "feature/f", "feature/b"} {
		if !contains(kept, want) {
			t.Errorf("kept is missing %q: %v", want, names(kept))
		}
	}
	if contains(kept, "feature/a") {
		t.Errorf("feature/a should have been evicted: %v", names(kept))
	}
}

func TestApplyRetentionNeverEvictsProtected(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	versions := []Version{
		{Name: "main", Protected: true, BuiltAt: base},
		{Name: "master", Protected: true, BuiltAt: base},
		{Name: "develop", Protected: true, BuiltAt: base},
	}
	// Even with a zero cap, protected branches stay.
	kept, evicted := applyRetention(versions, 0)
	if len(evicted) != 0 {
		t.Errorf("evicted = %v, want none", names(evicted))
	}
	if len(kept) != 3 {
		t.Errorf("kept = %v, want all 3 protected", names(kept))
	}
}

func names(vs []Version) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Name)
	}
	return out
}

func contains(vs []Version, name string) bool {
	for _, v := range vs {
		if v.Name == name {
			return true
		}
	}
	return false
}

func TestWriteIndexPageRespectsServiceOwnIndex(t *testing.T) {
	dir := t.TempDir()
	own := []byte("# Mine\n")
	if err := os.WriteFile(filepath.Join(dir, "index.md"), own, 0o644); err != nil {
		t.Fatal(err)
	}

	b := New(Config{CommonIndex: filepath.Join(t.TempDir(), "missing.md")})
	if err := b.writeIndexPage(dir, "svc", "main"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(own) {
		t.Errorf("service's own index.md was overwritten: %q", got)
	}
}

func TestWriteIndexPageRendersCommonTemplate(t *testing.T) {
	common := filepath.Join(t.TempDir(), "index.md")
	tmpl := "# {{ .Service }}\n\nbranch {{ .Branch }}\n" +
		"{{ range .Pages }}- [{{ .Title }}]({{ .File }})\n{{ end }}"
	if err := os.WriteFile(common, []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	// One page with an H1 (title comes from the heading) and one without
	// (title falls back to a tidied filename).
	files := map[string]string{
		"redis-entraid-auth-flow.md":  "# Redis Authentication via Microsoft Entra ID\n",
		"metadata_developer-guide.md": "no heading here\n",
		"notes.txt":                   "ignored\n",
	}
	for n, body := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	b := New(Config{CommonIndex: common})
	if err := b.writeIndexPage(dir, "mojro-common", "feature/redis-docs"); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatalf("no index.md generated: %v", err)
	}
	s := string(out)

	for _, want := range []string{
		"# mojro-common",
		"branch feature/redis-docs",
		"[Redis Authentication via Microsoft Entra ID](redis-entraid-auth-flow.md)",
		"[metadata developer guide](metadata_developer-guide.md)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("index.md missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "notes.txt") {
		t.Errorf("non-markdown file linked as a page:\n%s", s)
	}
}

func TestWriteIndexPageFallsBackWhenCommonMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "guide.md"), []byte("# Guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := New(Config{CommonIndex: filepath.Join(t.TempDir(), "nope.md")})
	if err := b.writeIndexPage(dir, "svc", "main"); err != nil {
		t.Fatalf("should not fail when the shared template is absent: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatalf("no index.md written: %v", err)
	}
	if !strings.Contains(string(out), "[Guide](guide.md)") {
		t.Errorf("minimal index missing page link:\n%s", out)
	}
}

func TestWriteRootIndexRendersLandingPage(t *testing.T) {
	site := t.TempDir()
	meta := t.TempDir()
	vl := VersionList{
		Service: "sample-api",
		Versions: []Version{
			{Name: "main", Protected: true, BuiltAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "feature/x", Protected: false, BuiltAt: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)},
		},
	}
	raw, err := json.Marshal(vl)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "sample-api.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	b := New(Config{SiteDir: site, MetaDir: meta})
	if err := b.WriteRootIndex(); err != nil {
		t.Fatal(err)
	}

	html, err := os.ReadFile(filepath.Join(site, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	for _, want := range []string{
		"Mojro documentation",
		"Sample API",
		"/sample-api/main/",
		"Planning &amp; optimization",
		"What’s new",
		"Videos",
		"/assets/img/mojro-wordmark.png",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("index.html missing %q", want)
		}
	}

	// See all must open the default version docs (original /service/branch/ target).
	if !strings.Contains(s, `href="/sample-api/main/"`) {
		t.Errorf("see-all / default docs link missing")
	}

	svcHTML, err := os.ReadFile(filepath.Join(site, "sample-api", "index.html"))
	if err != nil {
		t.Fatalf("service overview page missing: %v", err)
	}
	ss := string(svcHTML)
	for _, want := range []string{
		"Sample API",
		"/sample-api/main/",
		"/sample-api/feature/x/",
		"Developer Guide",
		"Versions",
	} {
		if !strings.Contains(ss, want) {
			t.Errorf("service page missing %q", want)
		}
	}

	for _, path := range []string{
		"assets/home/index.css",
		"assets/home/index.js",
		"assets/home/service.css",
		"assets/img/mojro-wordmark.png",
		"assets/img/mojro-logo.png",
	} {
		if _, err := os.Stat(filepath.Join(site, path)); err != nil {
			t.Errorf("expected asset %s: %v", path, err)
		}
	}
}

func TestDisplayName(t *testing.T) {
	cases := map[string]string{
		"shipper-api":  "Shipper API",
		"mojro-common": "Mojro Common",
		"auth-sdk":     "Auth SDK",
	}
	for in, want := range cases {
		if got := displayName(in); got != want {
			t.Errorf("displayName(%q) = %q, want %q", in, got, want)
		}
	}
}
