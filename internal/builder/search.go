package builder

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Search caps, so one request can't be turned into an expensive scan.
const (
	maxSearchTerms = 10
	maxQueryRunes  = 200
	snippetWidth   = 220
)

// SearchHit is one page — or one heading within a page — matching a query.
type SearchHit struct {
	Service     string `json:"service"`
	DisplayName string `json:"displayName"`
	Version     string `json:"version"`
	Title       string `json:"title"`
	// Page names the page a heading belongs to; empty when the hit is the page.
	Page    string  `json:"page,omitempty"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// SearchResults is the payload behind GET /api/search.
type SearchResults struct {
	Query string      `json:"query"`
	Terms []string    `json:"terms"`
	Total int         `json:"total"`
	Hits  []SearchHit `json:"hits"`
}

// searchDoc is one entry of a version's MkDocs search index, pre-lowered so a
// query doesn't re-lower the whole corpus on every keystroke.
type searchDoc struct {
	service     string
	displayName string
	version     string
	title       string
	page        string
	url         string
	text        string
	titleLower  string
	textLower   string
	anchor      bool
}

// searchShard is one version's parsed index, held until the file changes.
type searchShard struct {
	modTime time.Time
	size    int64
	docs    []searchDoc
}

// Search ranks documentation hits for a query across the default published
// version of every service. Older versions of the same service are skipped:
// including them would return the same page once per branch.
func (b *Builder) Search(query string, limit int) SearchResults {
	query = truncateRunes(strings.TrimSpace(query), maxQueryRunes)
	res := SearchResults{Query: query, Terms: []string{}, Hits: []SearchHit{}}

	terms := searchTerms(query)
	if len(terms) == 0 {
		return res
	}
	res.Terms = terms
	if limit <= 0 {
		limit = 20
	}

	phrase := asciiLower(query)
	hits := make([]SearchHit, 0, limit)
	for _, doc := range b.searchDocs() {
		score, ok := scoreDoc(doc, terms, phrase)
		if !ok {
			continue
		}
		hits = append(hits, SearchHit{
			Service:     doc.service,
			DisplayName: doc.displayName,
			Version:     doc.version,
			Title:       doc.title,
			Page:        doc.page,
			URL:         doc.url,
			Snippet:     snippet(doc, terms),
			Score:       score,
		})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Service != hits[j].Service {
			return hits[i].Service < hits[j].Service
		}
		return hits[i].Title < hits[j].Title
	})

	res.Total = len(hits)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	res.Hits = hits
	return res
}

// searchDocs returns the corpus, reloading any version whose index file changed
// since it was last read so a rebuild is picked up without a restart.
func (b *Builder) searchDocs() []searchDoc {
	targets := collectIndexServices(b.Services())

	b.searchMu.Lock()
	defer b.searchMu.Unlock()
	if b.searchShards == nil {
		b.searchShards = map[string]*searchShard{}
	}

	live := make(map[string]bool, len(targets))
	var out []searchDoc
	for _, svc := range targets {
		key := svc.Service + "/" + svc.DefaultName
		live[key] = true

		path := filepath.Join(b.cfg.SiteDir, svc.Service,
			filepath.FromSlash(svc.DefaultName), "search", "search_index.json")
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}

		shard := b.searchShards[key]
		if shard == nil || !shard.modTime.Equal(fi.ModTime()) || shard.size != fi.Size() {
			docs, err := loadSearchIndex(path, svc.Service, svc.DisplayName, svc.DefaultName)
			if err != nil {
				continue
			}
			shard = &searchShard{modTime: fi.ModTime(), size: fi.Size(), docs: docs}
			b.searchShards[key] = shard
		}
		out = append(out, shard.docs...)
	}

	// Drop versions that retention has since evicted.
	for key := range b.searchShards {
		if !live[key] {
			delete(b.searchShards, key)
		}
	}
	return out
}

// loadSearchIndex parses the search_index.json MkDocs writes for one build.
func loadSearchIndex(path, service, display, version string) ([]searchDoc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var idx struct {
		Docs []struct {
			Location string `json:"location"`
			Title    string `json:"title"`
			Text     string `json:"text"`
		} `json:"docs"`
	}
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, err
	}

	// Page titles keyed by location, so a heading hit can name its page.
	pageTitle := make(map[string]string, len(idx.Docs))
	for _, d := range idx.Docs {
		if !strings.Contains(d.Location, "#") {
			pageTitle[d.Location] = stripHTML(d.Title)
		}
	}

	base := "/" + service + "/" + version + "/"
	docs := make([]searchDoc, 0, len(idx.Docs))
	for _, d := range idx.Docs {
		// MkDocs stores rendered HTML in the index, so tags have to go before
		// matching: otherwise a search for "code" hits every <code> element.
		title := stripHTML(d.Title)
		text := stripHTML(d.Text)
		if title == "" && text == "" {
			continue
		}
		page := ""
		anchor := false
		if i := strings.Index(d.Location, "#"); i >= 0 {
			anchor = true
			page = pageTitle[d.Location[:i]]
		}
		docs = append(docs, searchDoc{
			service:     service,
			displayName: display,
			version:     version,
			title:       title,
			page:        page,
			url:         base + d.Location,
			text:        text,
			titleLower:  asciiLower(title),
			textLower:   asciiLower(text),
			anchor:      anchor,
		})
	}
	return docs, nil
}

// scoreDoc requires every term to appear somewhere in the doc, then weights
// title matches well above body matches. ok is false when the doc misses a term.
func scoreDoc(d searchDoc, terms []string, phrase string) (float64, bool) {
	score := 0.0
	for _, t := range terms {
		inTitle := strings.Contains(d.titleLower, t)
		n := strings.Count(d.textLower, t)
		if !inTitle && n == 0 {
			return 0, false
		}
		if inTitle {
			score += 12
			if strings.HasPrefix(d.titleLower, t) {
				score += 6
			}
		}
		if n > 5 {
			n = 5 // a long page repeating a word isn't 50x more relevant
		}
		score += float64(n)
	}
	if len(terms) > 1 {
		if strings.Contains(d.titleLower, phrase) {
			score += 40
		}
		if strings.Contains(d.textLower, phrase) {
			score += 12
		}
	}
	if d.titleLower == phrase {
		score += 60
	}
	if !d.anchor {
		score += 3 // prefer the page over one of its own sections
	}
	return score, true
}

// snippet returns a window of body text around the first matching term.
func snippet(d searchDoc, terms []string) string {
	if d.text == "" {
		return ""
	}
	at := -1
	for _, t := range terms {
		if i := strings.Index(d.textLower, t); i >= 0 && (at < 0 || i < at) {
			at = i
		}
	}
	if at < 0 {
		// Term matched the title only — lead with the start of the page.
		if len(d.text) <= snippetWidth {
			return d.text
		}
		return clipAtSpace(d.text, snippetWidth) + "…"
	}

	start := at - snippetWidth/3
	if start < 0 {
		start = 0
	}
	end := start + snippetWidth
	if end > len(d.text) {
		end = len(d.text)
	}
	// Snap both edges to spaces, which keeps whole words and, because a space is
	// one byte, can never split a multi-byte rune.
	if start > 0 {
		if sp := strings.IndexByte(d.text[start:end], ' '); sp >= 0 {
			start += sp + 1
		}
	}
	if end < len(d.text) {
		if sp := strings.LastIndexByte(d.text[start:end], ' '); sp > 0 {
			end = start + sp
		}
	}

	out := strings.TrimSpace(d.text[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(d.text) {
		out += "…"
	}
	return out
}

// searchTerms splits a query into deduplicated lowercase terms.
func searchTerms(q string) []string {
	fields := strings.FieldsFunc(asciiLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, f := range fields {
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
		if len(out) == maxSearchTerms {
			break
		}
	}
	return out
}

// asciiLower lowercases A-Z and leaves every other byte alone. Unlike
// strings.ToLower it never changes byte length, so an offset found in the
// lowered copy also indexes the original — which snippet extraction relies on.
func asciiLower(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if b == nil {
				b = []byte(s)
			}
			b[i] = c + ('a' - 'A')
		}
	}
	if b == nil {
		return s
	}
	return string(b)
}

// stripHTML removes tags from MkDocs' rendered index text and resolves the
// entities left behind, so both matching and snippets see plain prose.
func stripHTML(s string) string {
	if !strings.ContainsAny(s, "<&") {
		return collapseSpace(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
			// A tag boundary separates words: "a<br>b" must not become "ab".
			b.WriteByte(' ')
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteByte(s[i])
			}
		}
	}
	return collapseSpace(html.UnescapeString(b.String()))
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func clipAtSpace(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if sp := strings.LastIndexByte(s[:n], ' '); sp > 0 {
		return strings.TrimSpace(s[:sp])
	}
	// No space to snap to: back off to a rune boundary.
	for n > 0 && !utf8Start(s[n]) {
		n--
	}
	return strings.TrimSpace(s[:n])
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s // byte length bounds rune count
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}
