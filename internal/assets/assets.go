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

// Logo is the Mojro mark shown in each site's header and as the favicon.
//
//go:embed mojro-logo.png
var Logo []byte
