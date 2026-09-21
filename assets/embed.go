// Package assets embeds the fonts shipped with the binary.
package assets

import "embed"

// Fonts holds the bundled Noto fonts and their license.
//
//go:embed fonts/NotoSansCJKsc-Regular.otf fonts/NotoSansCJKsc-Bold.otf fonts/NotoEmoji-VariableFont_wght.ttf fonts/NotoEmoji-LICENSE.txt
var Fonts embed.FS
