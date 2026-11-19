// Package private_drop embeds the built frontend so the server ships as a
// single binary. The file lives at the module root because go:embed patterns
// cannot reference parent directories.
package private_drop

import "embed"

// WebFS contains web/dist as built by the frontend toolchain.
//
//go:embed web/dist
var WebFS embed.FS
