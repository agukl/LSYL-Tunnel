package front

import "embed"

// FS contains the standard Windows client web assets.
//
//go:embed index.html styles.css app.js
var FS embed.FS
