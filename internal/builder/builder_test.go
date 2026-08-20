package builder

import (
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
