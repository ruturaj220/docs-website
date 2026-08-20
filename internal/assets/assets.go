// Package assets embeds the shared theme files injected into every built site,
// so service repos only ever contain their own .md files.
package assets

import _ "embed"

// VersionSelectorJS renders the version flyout by reading versions.json.
//
//go:embed version-selector.js
var VersionSelectorJS []byte

// VersionSelectorCSS styles the flyout.
//
//go:embed version-selector.css
var VersionSelectorCSS []byte

// ThemeCSS applies Mojro brand colors to the Material for MkDocs theme.
//
//go:embed theme.css
var ThemeCSS []byte

// Logo is the Mojro mark shown in each site's header and as the favicon.
//
//go:embed mojro-logo.png
var Logo []byte

// Wordmark is the Mojro logo used on the root landing page.
//
//go:embed mojro-wordmark.png
var Wordmark []byte

// IndexHTML is the Go html/template for the root landing page at /.
//
//go:embed index.html
var IndexHTML []byte

// IndexCSS styles the root landing page.
//
//go:embed index.css
var IndexCSS []byte

// IndexJS powers search and header behavior on the root landing page.
//
//go:embed index.js
var IndexJS []byte

// ServiceHTML is the Go html/template for /<service>/ (See all).
//
//go:embed service.html
var ServiceHTML []byte

// ServiceCSS styles the service overview page.
//
//go:embed service.css
var ServiceCSS []byte
