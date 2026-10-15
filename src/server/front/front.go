package front

import "embed"

// FS contains the server operation console web assets.
//
//go:embed index.html styles.css app.js modules/*.js
var FS embed.FS
