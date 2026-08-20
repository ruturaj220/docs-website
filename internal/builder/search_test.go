package builder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSearchTerms(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"Redis", []string{"redis"}},
		{"feature/redis-docs", []string{"feature", "redis", "docs"}},
		{"redis REDIS redis", []string{"redis"}},
		{"!!! ???", nil},
		{"MetaData enum", []string{"metadata", "enum"}},
	}
	for _, tc := range tests {
		got := searchTerms(tc.in)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("searchTerms(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSearchTermsCapped(t *testing.T) {
	got := searchTerms("a b c d e f g h i j k l m n")
	if len(got) != maxSearchTerms {
		t.Fatalf("got %d terms, want cap of %d", len(got), maxSearchTerms)
	}
}

func TestAsciiLowerPreservesByteLength(t *testing.T) {
	for _, s := range []string{"Redis", "MetaData — enum", "ünïcode ÄÖÜ", ""} {
		if got := asciiLower(s); len(got) != len(s) {
			t.Errorf("asciiLower(%q): length %d, want %d", s, len(got), len(s))
		}
	}
	if got := asciiLower("MixedCASE"); got != "mixedcase" {
		t.Errorf("asciiLower: got %q", got)
	}
}

func TestScoreDocRequiresEveryTerm(t *testing.T) {
	doc := newDoc("Redis auth", "Configure Redis authentication for the cluster.")

	if _, ok := scoreDoc(doc, []string{"redis"}, "redis"); !ok {
		t.Error("term present in title should match")
	}
	if _, ok := scoreDoc(doc, []string{"cluster"}, "cluster"); !ok {
		t.Error("term present in body should match")
	}
	if _, ok := scoreDoc(doc, []string{"redis", "kafka"}, "redis kafka"); ok {
		t.Error("a missing term must reject the doc")
	}
}

func TestScoreDocRanksTitleOverBody(t *testing.T) {
	inTitle := newDoc("Redis authentication", "Nothing else here.")
	inBody := newDoc("Cluster setup", "You may configure redis if you want to.")

	title, _ := scoreDoc(inTitle, []string{"redis"}, "redis")
	body, _ := scoreDoc(inBody, []string{"redis"}, "redis")
	if title <= body {
		t.Errorf("title hit (%v) should outrank body hit (%v)", title, body)
	}
}

func TestScoreDocPrefersPageOverItsAnchor(t *testing.T) {
	page := newDoc("Redis", "Redis notes.")
	anchor := newDoc("Redis", "Redis notes.")
	anchor.anchor = true

	p, _ := scoreDoc(page, []string{"redis"}, "redis")
	a, _ := scoreDoc(anchor, []string{"redis"}, "redis")
	if p <= a {
		t.Errorf("page (%v) should outrank its own anchor (%v)", p, a)
	}
}

func TestScoreDocRepeatedTermIsCapped(t *testing.T) {
	few := newDoc("Notes", "redis redis")
	many := newDoc("Notes", strings.Repeat("redis ", 60))

	f, _ := scoreDoc(few, []string{"redis"}, "redis")
	m, _ := scoreDoc(many, []string{"redis"}, "redis")
	if m-f > 5 {
		t.Errorf("repetition gained %v points; cap should hold it to 5", m-f)
	}
}

func TestSnippetWindowsAroundMatch(t *testing.T) {
	body := strings.Repeat("filler words here. ", 40) + "the redis token expires hourly. " +
		strings.Repeat("more filler. ", 40)
	doc := newDoc("Notes", body)

	got := snippet(doc, []string{"redis"})
	if !strings.Contains(got, "redis token expires") {
		t.Errorf("snippet lost the match: %q", got)
	}
	if len(got) > snippetWidth+16 { // +ellipses and word snapping
		t.Errorf("snippet too long (%d bytes): %q", len(got), got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipses on both sides: %q", got)
	}
}

func TestSnippetTitleOnlyMatchFallsBackToLead(t *testing.T) {
	doc := newDoc("Redis", "This body never mentions the search term at all.")
	got := snippet(doc, []string{"redis"})
	if !strings.HasPrefix(got, "This body never") {
		t.Errorf("expected the page lead, got %q", got)
	}
}

func TestSnippetKeepsUTF8Valid(t *testing.T) {
	// Multi-byte runes on both sides of the match, so a naive byte slice would
	// cut one in half.
	body := strings.Repeat("héllo wörld — ", 40) + "redis " + strings.Repeat("ünïcode ", 40)
	got := snippet(newDoc("Notes", body), []string{"redis"})
	for i, r := range got {
		if r == '�' {
			t.Fatalf("snippet has an invalid rune at %d: %q", i, got)
		}
	}
}

func TestSearchEndToEnd(t *testing.T) {
	b := builderWithIndex(t)

	res := b.Search("redis", 10)
	if res.Total == 0 {
		t.Fatal("expected hits for \"redis\"")
	}
	if res.Hits[0].Title != "Redis auth" {
		t.Errorf("top hit = %q, want the title match \"Redis auth\"", res.Hits[0].Title)
	}
	if got := res.Hits[0].URL; got != "/svc-a/main/redis-auth/" {
		t.Errorf("URL = %q", got)
	}
	if res.Hits[0].DisplayName != "Svc A" {
		t.Errorf("DisplayName = %q", res.Hits[0].DisplayName)
	}
	if strings.Join(res.Terms, ",") != "redis" {
		t.Errorf("Terms = %v", res.Terms)
	}
}

func TestSearchAnchorNamesItsPage(t *testing.T) {
	b := builderWithIndex(t)
	res := b.Search("rotation", 10)
	if res.Total == 0 {
		t.Fatal("expected a hit for the heading")
	}
	hit := res.Hits[0]
	if hit.Page != "Redis auth" {
		t.Errorf("Page = %q, want the parent page title", hit.Page)
	}
	if !strings.Contains(hit.URL, "#") {
		t.Errorf("expected an anchor URL, got %q", hit.URL)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	b := builderWithIndex(t)
	for _, q := range []string{"", "   ", "!!!"} {
		res := b.Search(q, 10)
		if res.Total != 0 || len(res.Hits) != 0 {
			t.Errorf("Search(%q) returned %d hits, want none", q, res.Total)
		}
		if res.Hits == nil || res.Terms == nil {
			t.Errorf("Search(%q): slices must marshal as [] not null", q)
		}
	}
}

func TestSearchNoMatch(t *testing.T) {
	b := builderWithIndex(t)
	res := b.Search("kubernetes", 10)
	if res.Total != 0 {
		t.Errorf("expected no hits, got %d", res.Total)
	}
}

func TestSearchRespectsLimit(t *testing.T) {
	b := builderWithIndex(t)
	res := b.Search("docs", 1)
	if len(res.Hits) > 1 {
		t.Errorf("limit ignored: %d hits", len(res.Hits))
	}
	if res.Total < len(res.Hits) {
		t.Errorf("Total (%d) must count matches before the limit", res.Total)
	}
}

func TestSearchReloadsWhenIndexChanges(t *testing.T) {
	b := builderWithIndex(t)
	if b.Search("redis", 10).Total == 0 {
		t.Fatal("expected an initial hit")
	}

	// Rewrite the index with different content, as a rebuild would.
	writeIndex(t, b, "svc-a", "main", []indexDoc{
		{Location: "kafka/", Title: "Kafka setup", Text: "Broker configuration."},
	})
	if got := b.Search("redis", 10).Total; got != 0 {
		t.Errorf("stale cache: still got %d hits for the removed page", got)
	}
	if got := b.Search("kafka", 10).Total; got == 0 {
		t.Error("new content not picked up")
	}
}

// --- helpers ---------------------------------------------------------------

func newDoc(title, text string) searchDoc {
	return searchDoc{
		title:      title,
		text:       text,
		titleLower: asciiLower(title),
		textLower:  asciiLower(text),
	}
}

type indexDoc struct {
	Location string `json:"location"`
	Title    string `json:"title"`
	Text     string `json:"text"`
}

// builderWithIndex returns a Builder backed by a temp site tree holding one
// service at main with a small MkDocs-shaped search index.
func builderWithIndex(t *testing.T) *Builder {
	t.Helper()
	dir := t.TempDir()
	b := New(Config{
		SiteDir:       filepath.Join(dir, "site"),
		MetaDir:       filepath.Join(dir, "meta"),
		ProtectedRefs: []string{"main"},
	})

	if err := os.MkdirAll(b.cfg.MetaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	vl := VersionList{
		Service:  "svc-a",
		Versions: []Version{{Name: "main", Protected: true, BuiltAt: time.Now().UTC()}},
	}
	raw, err := json.MarshalIndent(vl, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.cfg.MetaDir, "svc-a.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	writeIndex(t, b, "svc-a", "main", []indexDoc{
		{Location: "", Title: "Svc A", Text: "Landing page for the docs."},
		{Location: "redis-auth/", Title: "Redis auth", Text: "How to configure redis credentials in the docs."},
		{Location: "redis-auth/#rotation", Title: "Rotation", Text: "Rotate the token every hour."},
		{Location: "other/", Title: "Something else", Text: "Unrelated docs content."},
	})
	if len(b.Services()) == 0 {
		t.Fatal("fixture produced no services; check the meta layout")
	}
	return b
}

func writeIndex(t *testing.T, b *Builder, service, version string, docs []indexDoc) {
	t.Helper()
	dir := filepath.Join(b.cfg.SiteDir, service, filepath.FromSlash(version), "search")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"config": map[string]any{}, "docs": docs})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "search_index.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	// Make the change unambiguous to the mtime-based cache.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain text", "plain text"},
		{"<p>Hello</p>", "Hello"},
		{"a<br>b", "a b"},
		{"<code>MetaData.java</code>", "MetaData.java"},
		{"2. Declare it in <code>X</code> now", "2. Declare it in X now"},
		{"&lt;p&gt; stays visible", "<p> stays visible"},
		{"&amp; and &quot;quotes&quot;", "& and \"quotes\""},
		{"<p>  spaced   out  </p>", "spaced out"},
		{"", ""},
		{"<a href=\"/x?y=1&z=2\">link</a>", "link"},
	}
	for _, tc := range tests {
		if got := stripHTML(tc.in); got != tc.want {
			t.Errorf("stripHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSearchIgnoresMarkup(t *testing.T) {
	b := builderWithIndex(t)
	// A page whose only "code" is the <code> tag itself must not match "code".
	writeIndex(t, b, "svc-a", "main", []indexDoc{
		{Location: "a/", Title: "Config", Text: "<p>Set <code>timeout</code> to 30s.</p>"},
	})

	if got := b.Search("code", 10).Total; got != 0 {
		t.Errorf("matched the <code> tag: %d hits", got)
	}
	res := b.Search("timeout", 10)
	if res.Total != 1 {
		t.Fatalf("expected the real word to match, got %d", res.Total)
	}
	if strings.ContainsAny(res.Hits[0].Snippet, "<>") {
		t.Errorf("snippet still carries markup: %q", res.Hits[0].Snippet)
	}
	if res.Hits[0].Snippet != "Set timeout to 30s." {
		t.Errorf("snippet = %q", res.Hits[0].Snippet)
	}
}

func TestSearchStripsMarkupFromTitles(t *testing.T) {
	b := builderWithIndex(t)
	writeIndex(t, b, "svc-a", "main", []indexDoc{
		{Location: "a/", Title: "Page", Text: "Body."},
		{Location: "a/#step", Title: "Declare it in <code>MetaData.java</code>", Text: "Add the constant."},
	})
	res := b.Search("metadata", 10)
	if res.Total == 0 {
		t.Fatal("expected a hit")
	}
	if got := res.Hits[0].Title; got != "Declare it in MetaData.java" {
		t.Errorf("Title = %q, want markup stripped", got)
	}
}
