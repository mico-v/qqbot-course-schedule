// Package render draws the course schedule cards as JPEG images.
package render

import (
	"fmt"
	"strings"

	"github.com/fogleman/gg"
	"github.com/rivo/uniseg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"

	"github.com/mico-v/qqbot-course-schedule/assets"
)

// FontSet holds the parsed fonts shared by every render.
type FontSet struct {
	regular *sfnt.Font
	bold    *sfnt.Font
	emoji   *sfnt.Font
}

// LoadFonts parses the embedded fonts once.
func LoadFonts() (*FontSet, error) {
	regular, err := parseFont("fonts/NotoSansCJKsc-Regular.otf")
	if err != nil {
		return nil, err
	}
	bold, err := parseFont("fonts/NotoSansCJKsc-Bold.otf")
	if err != nil {
		return nil, err
	}
	emoji, err := parseFont("fonts/NotoEmoji-VariableFont_wght.ttf")
	if err != nil {
		return nil, err
	}
	return &FontSet{regular: regular, bold: bold, emoji: emoji}, nil
}

func parseFont(name string) (*sfnt.Font, error) {
	raw, err := assets.Fonts.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("读取字体 %s 失败: %w", name, err)
	}
	parsed, err := opentype.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("解析字体 %s 失败: %w", name, err)
	}
	return parsed, nil
}

type faceKey struct {
	size int
	bold bool
}

type cachedFace struct {
	face font.Face
	font *sfnt.Font
}

type faceCache struct {
	fonts *FontSet
	faces map[faceKey]cachedFace
	buf   sfnt.Buffer
}

func newFaceCache(fonts *FontSet) *faceCache {
	return &faceCache{fonts: fonts, faces: make(map[faceKey]cachedFace)}
}

func (fc *faceCache) get(size int, bold bool) cachedFace {
	key := faceKey{size: size, bold: bold}
	if cached, ok := fc.faces[key]; ok {
		return cached
	}
	raw := fc.fonts.regular
	if bold {
		raw = fc.fonts.bold
	}
	face, err := opentype.NewFace(raw, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		// Fall back to the regular face; fonts are embedded so this is rare.
		face, _ = opentype.NewFace(fc.fonts.regular, &opentype.FaceOptions{Size: float64(size), DPI: 72})
		raw = fc.fonts.regular
	}
	cached := cachedFace{face: face, font: raw}
	fc.faces[key] = cached
	return cached
}

// supports reports whether the font has a real glyph for r.
func (fc *faceCache) supports(raw *sfnt.Font, r rune) bool {
	index, err := raw.GlyphIndex(&fc.buf, r)
	return err == nil && index != 0
}

func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF:
		return true
	case r >= 0x2600 && r <= 0x27BF:
		return true
	case r >= 0x1F1E6 && r <= 0x1F1FF:
		return true
	case r == 0xFE0F || r == 0x200D:
		return true
	case r >= 0x2190 && r <= 0x21FF:
		return true
	case r >= 0x2B00 && r <= 0x2BFF:
		return true
	default:
		return false
	}
}

// faceForCluster picks the CJK face when it covers the cluster, otherwise the
// monochrome emoji face, and falls back to CJK so text never disappears.
func (fc *faceCache) faceForCluster(cluster string, size int, bold bool) cachedFace {
	candidate := fc.get(size, bold)
	missing := false
	for _, r := range cluster {
		if r == 0xFE0F || r == 0x200D {
			continue
		}
		if !fc.supports(candidate.font, r) {
			missing = true
			break
		}
	}
	if !missing {
		return candidate
	}
	emojiCandidate := cachedFace{face: mustFace(fc.fonts.emoji, size), font: fc.fonts.emoji}
	covered := true
	for _, r := range cluster {
		if r == 0xFE0F || r == 0x200D {
			continue
		}
		if !fc.supports(fc.fonts.emoji, r) {
			covered = false
			break
		}
	}
	if covered || isEmojiCluster(cluster) {
		return emojiCandidate
	}
	return candidate
}

func mustFace(raw *sfnt.Font, size int) font.Face {
	face, err := opentype.NewFace(raw, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		face, _ = opentype.NewFace(raw, &opentype.FaceOptions{Size: float64(size), DPI: 72})
	}
	return face
}

func isEmojiCluster(cluster string) bool {
	for _, r := range cluster {
		if isEmojiRune(r) {
			return true
		}
	}
	return false
}

// graphemes splits text into user-perceived characters.
func graphemes(text string) []string {
	if text == "" {
		return nil
	}
	iterator := uniseg.NewGraphemes(text)
	var clusters []string
	for iterator.Next() {
		clusters = append(clusters, iterator.Str())
	}
	return clusters
}

// measureText returns the pixel width of text.
func (fc *faceCache) measureText(text string, size int, bold bool) float64 {
	width := 0.0
	for _, cluster := range graphemes(text) {
		width += fc.clusterWidth(cluster, size, bold)
	}
	return width
}

func (fc *faceCache) clusterWidth(cluster string, size int, bold bool) float64 {
	cached := fc.faceForCluster(cluster, size, bold)
	return float64(font.MeasureString(cached.face, cluster)) / 64
}

// fitText truncates text with an ellipsis so it fits maxWidth.
func (fc *faceCache) fitText(text string, size int, maxWidth float64, bold bool) string {
	if maxWidth <= 0 || fc.measureText(text, size, bold) <= maxWidth {
		return text
	}
	ellipsis := "…"
	limit := maxWidth - fc.measureText(ellipsis, size, bold)
	width := 0.0
	var builder strings.Builder
	for _, cluster := range graphemes(text) {
		clusterWidth := fc.clusterWidth(cluster, size, bold)
		if width+clusterWidth > limit {
			break
		}
		builder.WriteString(cluster)
		width += clusterWidth
	}
	return builder.String() + ellipsis
}

// wrapText greedily wraps text to maxWidth, keeping explicit newlines.
func (fc *faceCache) wrapText(text string, size int, maxWidth float64, bold bool) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		var current strings.Builder
		currentWidth := 0.0
		for _, cluster := range graphemes(paragraph) {
			clusterWidth := fc.clusterWidth(cluster, size, bold)
			if currentWidth+clusterWidth > maxWidth && current.Len() > 0 {
				lines = append(lines, current.String())
				current.Reset()
				currentWidth = 0
			}
			current.WriteString(cluster)
			currentWidth += clusterWidth
		}
		lines = append(lines, current.String())
	}
	return lines
}

// drawText draws text with its top edge at (x, top), batching consecutive
// clusters that share a face into a single DrawString call.
func (fc *faceCache) drawText(dc *gg.Context, x, top float64, text string, size int, colorHex string, bold bool, maxWidth float64) {
	if text == "" {
		return
	}
	if maxWidth > 0 {
		text = fc.fitText(text, size, maxWidth, bold)
	}
	dc.SetHexColor(colorHex)
	cursor := x
	var batch strings.Builder
	var batchFace cachedFace
	flush := func() {
		if batch.Len() == 0 {
			return
		}
		dc.SetFontFace(batchFace.face)
		baseline := top + float64(batchFace.face.Metrics().Ascent)/64
		dc.DrawString(batch.String(), cursor, baseline)
		cursor += fc.measureText(batch.String(), size, bold)
		batch.Reset()
	}
	for _, cluster := range graphemes(text) {
		cached := fc.faceForCluster(cluster, size, bold)
		if batch.Len() > 0 && cached.face != batchFace.face {
			flush()
		}
		batchFace = cached
		batch.WriteString(cluster)
	}
	flush()
}

// drawTextCentered draws text horizontally centered inside [left, right].
func (fc *faceCache) drawTextCentered(dc *gg.Context, left, right, top float64, text string, size int, colorHex string, bold bool) {
	width := fc.measureText(text, size, bold)
	fc.drawText(dc, left+(right-left-width)/2, top, text, size, colorHex, bold, 0)
}
