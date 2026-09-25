package render

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"

	"github.com/fogleman/gg"
	xdraw "golang.org/x/image/draw"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

const (
	cardWidth        = 1240
	headerHeight     = 202
	cardGap          = 16
	footerHeight     = 54
	cardLeft         = 36
	nameWidth        = 235
	nameLineHeight   = 30
	avatarSize       = 76
	foldedAvatarSize = 40
	foldedCellWidth  = 214
	foldedNameWidth  = 150
	foldedRowHeight  = 62
	foldedPadX       = 30
	foldedPadY       = 22
	foldedTitleH     = 40
)

type statusColors struct {
	foreground string
	badge      string
	accent     string
}

var statusColorMap = map[string]statusColors{
	"active":    {"#0f766e", "#ccfbf1", "#14b8a6"},
	"upcoming":  {"#2563eb", "#dbeafe", "#60a5fa"},
	"finished":  {"#64748b", "#f1f5f9", "#94a3b8"},
	"scheduled": {"#7c3aed", "#ede9fe", "#a78bfa"},
	"holiday":   {"#b45309", "#fef3c7", "#f59e0b"},
	"none":      {"#64748b", "#f8fafc", "#cbd5e1"},
	"rank1":     {"#b45309", "#fef3c7", "#f59e0b"},
	"rank2":     {"#475569", "#e2e8f0", "#94a3b8"},
	"rank3":     {"#9a3412", "#ffedd5", "#fb923c"},
	"rank":      {"#1d4ed8", "#dbeafe", "#60a5fa"},
}

func statusColorsFor(key string) statusColors {
	if colors, ok := statusColorMap[key]; ok {
		return colors
	}
	return statusColorMap["none"]
}

// Renderer draws cards using the shared embedded fonts.
type Renderer struct {
	fonts *FontSet
}

// New loads the fonts once and returns a renderer.
func New() (*Renderer, error) {
	fonts, err := LoadFonts()
	if err != nil {
		return nil, err
	}
	return &Renderer{fonts: fonts}, nil
}

// LegendItem is one colour legend entry; Key selects the accent colour.
type LegendItem struct {
	Key   string
	Label string
}

// DayCardData is the input for one day view (also used by the rank board).
type DayCardData struct {
	Title       string
	Subtitle    string
	FoldedTitle string
	Footer      string
	Rows        []schedule.DayRow
	Folded      []schedule.DayRow
	BotAvatar   image.Image
	// Avatars maps user_id to a real avatar; missing ids fall back to initials.
	Avatars map[string]image.Image
	// Legend overrides the auto-generated legend when non-nil.
	Legend []LegendItem
	// DurationLabel prefixes the duration line; empty means no prefix.
	DurationLabel string
}

// DayCard renders the daily schedule card.
func (r *Renderer) DayCard(data DayCardData) image.Image {
	fc := newFaceCache(r.fonts)

	type nameBlock struct {
		lines  []string
		height int
	}
	nameBlocks := make([]nameBlock, len(data.Rows))
	for index, row := range data.Rows {
		lines := fc.wrapText(displayName(row), 27, nameWidth, true)
		height := 156
		if extra := (len(lines) - 1) * nameLineHeight; extra > 0 {
			height += extra
		}
		nameBlocks[index] = nameBlock{lines: lines, height: height}
	}
	cardsHeight := 0
	for index, block := range nameBlocks {
		cardsHeight += block.height
		if index > 0 {
			cardsHeight += cardGap
		}
	}

	innerWidth := cardWidth - cardLeft*2
	foldedHeight := foldedStripHeight(len(data.Folded), innerWidth)
	foldedGap := 0
	if len(data.Rows) > 0 && len(data.Folded) > 0 {
		foldedGap = cardGap
	}
	bodyHeight := 140
	if len(data.Rows) > 0 || len(data.Folded) > 0 {
		bodyHeight = cardsHeight + foldedGap + foldedHeight
	}
	height := maxInt(360, headerHeight+bodyHeight+footerHeight)

	dc := gg.NewContext(cardWidth, height)
	dc.SetHexColor("#f5f7fc")
	dc.Clear()

	// Header gradient with two decorative circles.
	headerTop := color.RGBA{30, 41, 72, 255}
	headerBottom := color.RGBA{48, 73, 116, 255}
	for y := 0; y < headerHeight; y++ {
		ratio := float64(y) / float64(headerHeight-1)
		mixed := color.RGBA{
			R: uint8(float64(headerTop.R) + (float64(headerBottom.R)-float64(headerTop.R))*ratio),
			G: uint8(float64(headerTop.G) + (float64(headerBottom.G)-float64(headerTop.G))*ratio),
			B: uint8(float64(headerTop.B) + (float64(headerBottom.B)-float64(headerTop.B))*ratio),
			A: 255,
		}
		dc.SetColor(mixed)
		dc.DrawRectangle(0, float64(y), cardWidth, 1)
		dc.Fill()
	}
	dc.SetHexColor("#496a9d")
	dc.DrawEllipse(cardWidth-190+130, -105+130, 130, 130)
	dc.Fill()
	dc.SetHexColor("#3e5b8d")
	dc.DrawEllipse(cardWidth-90, 70+90, 90, 90)
	dc.Fill()

	fc.drawText(dc, 42, 32, data.Title, 40, "#ffffff", true, 760)
	subtitle := data.Subtitle
	if subtitle == "" {
		activeCount, upcomingCount := 0, 0
		for _, row := range data.Rows {
			switch row.StatusKey {
			case "active":
				activeCount++
			case "upcoming", "scheduled":
				upcomingCount++
			}
		}
		subtitle = fmt.Sprintf("共 %d 位成员  ·  %d 人正在上课  ·  %d 人待上课",
			len(data.Rows)+len(data.Folded), activeCount, upcomingCount)
	}
	fc.drawText(dc, 44, 92, subtitle, 20, "#dbeafe", false, 1000)

	if data.BotAvatar != nil {
		dc.DrawImage(circleImage(data.BotAvatar, 88), cardWidth-172, 57)
	}

	legend := data.Legend
	if legend == nil {
		legend = legendItems(data.Rows)
	}
	legendX := 44.0
	for _, item := range legend {
		colors := statusColorsFor(item.Key)
		dc.SetHexColor(colors.accent)
		dc.DrawCircle(legendX+5, 143+15, 5)
		dc.Fill()
		fc.drawText(dc, legendX+18, 143, item.Label, 17, "#e2e8f0", false, 0)
		legendX += fc.measureText(item.Label, 17, false) + 58
	}

	if len(data.Rows) == 0 && len(data.Folded) == 0 {
		dc.SetHexColor("#ffffff")
		dc.DrawRoundedRectangle(cardLeft, headerHeight, cardWidth-cardLeft*2, 140, 22)
		dc.Fill()
		fc.drawTextCentered(dc, cardLeft, cardWidth-cardLeft, headerHeight+52, "暂无成员课程数据", 20, "#64748b", false)
	}

	top := float64(headerHeight)
	for index, row := range data.Rows {
		block := nameBlocks[index]
		height := float64(block.height)
		right := float64(cardWidth - cardLeft)
		colors := statusColorsFor(row.StatusKey)
		cardFill := "#ffffff"
		if index%2 == 1 {
			cardFill = "#fcfdff"
		}
		dc.SetHexColor(cardFill)
		dc.DrawRoundedRectangle(cardLeft, top, right-cardLeft, height, 22)
		dc.Fill()
		dc.SetHexColor(colors.accent)
		dc.DrawRoundedRectangle(cardLeft, top, 8, height, 4)
		dc.Fill()

		dc.DrawImage(fc.memberAvatar(row, data.Avatars, avatarSize), 62, int(top)+40)
		nameTop := top + 31
		for lineIndex, line := range block.lines {
			fc.drawText(dc, 158, nameTop+float64(lineIndex*nameLineHeight), line, 27, "#17233c", true, nameWidth)
		}

		courseColor := "#17233c"
		if row.StatusKey == "none" {
			courseColor = "#64748b"
		}
		fc.drawText(dc, 430, top+25, row.Course, 26, courseColor, true, 470)
		timeText := row.TimeText
		if location := strings.TrimSpace(row.Location); location != "" {
			timeText += "   ·   " + location
		}
		fc.drawText(dc, 430, top+66, timeText, 19, "#64748b", false, 480)
		durationText := row.Duration
		if data.DurationLabel != "" {
			durationText = data.DurationLabel + " " + durationText
		}
		fc.drawText(dc, 430, top+101, durationText, 17, "#94a3b8", false, 480)
		drawProgress(dc, 430, top+132, 480, row.Progress, colors.accent)

		drawBadge(dc, fc, 972, top+24, row.Status, colors.foreground, colors.badge)
		fc.drawText(dc, 972, top+76, row.CountdownLabel, 17, "#94a3b8", false, 0)
		fc.drawText(dc, 972, top+96, row.Countdown, 20, colors.foreground, true, right-972-24)

		top += height + cardGap
	}

	if len(data.Rows) > 0 {
		top -= cardGap
	}
	if len(data.Folded) > 0 {
		top += float64(foldedGap)
		drawFoldedStrip(dc, fc, data.Folded, data.Avatars, cardLeft, top, innerWidth, data.FoldedTitle)
		top += float64(foldedHeight)
	}

	footerTop := top
	if len(data.Rows) == 0 && len(data.Folded) == 0 {
		footerTop = headerHeight + 140
	}
	fc.drawTextCentered(dc, 0, cardWidth, footerTop+22-8, data.Footer, 17, "#94a3b8", false)

	return dc.Image()
}

func legendItems(rows []schedule.DayRow) []LegendItem {
	present := make(map[string]bool, len(rows))
	hasOverride := false
	for _, row := range rows {
		present[row.StatusKey] = true
		if row.OverrideNote != "" {
			hasOverride = true
		}
	}
	var items []LegendItem
	for _, candidate := range []LegendItem{
		{"active", "正在上课"},
		{"upcoming", "下一节即将上"},
		{"finished", "今日已结束"},
	} {
		if present[candidate.Key] {
			items = append(items, candidate)
		}
	}
	if present["holiday"] {
		items = append(items, LegendItem{"holiday", "休假"})
	}
	if hasOverride {
		items = append(items, LegendItem{"scheduled", "调休上课"})
	}
	return items
}

func drawBadge(dc *gg.Context, fc *faceCache, left, top float64, text, foreground, background string) {
	if text == "" {
		return
	}
	width := fc.measureText(text, 16, true) + 32
	dc.SetHexColor(background)
	dc.DrawRoundedRectangle(left, top, width, 34, 17)
	dc.Fill()
	fc.drawText(dc, left+16, top+5, text, 16, foreground, true, 0)
}

func drawProgress(dc *gg.Context, left, top, width, progress float64, colorHex string) {
	height := 8.0
	dc.SetHexColor("#e2e8f0")
	dc.DrawRoundedRectangle(left, top, width, height, 4)
	dc.Fill()
	fillWidth := math.Min(1, math.Max(0, progress)) * width
	if progress > 0 && fillWidth < 8 {
		fillWidth = 8
	}
	if fillWidth > 0 {
		dc.SetHexColor(colorHex)
		dc.DrawRoundedRectangle(left, top, fillWidth, height, 4)
		dc.Fill()
	}
}

func foldedColumns(innerWidth int) int {
	columns := innerWidth / foldedCellWidth
	if columns < 1 {
		return 1
	}
	return columns
}

func foldedStripHeight(count, innerWidth int) int {
	if count <= 0 {
		return 0
	}
	columns := foldedColumns(innerWidth)
	rows := (count + columns - 1) / columns
	return foldedPadY*2 + foldedTitleH + rows*foldedRowHeight
}

func drawFoldedStrip(dc *gg.Context, fc *faceCache, rows []schedule.DayRow, avatars map[string]image.Image, left, top float64, innerWidth int, title string) {
	if len(rows) == 0 {
		return
	}
	height := float64(foldedStripHeight(len(rows), innerWidth))
	dc.SetHexColor("#ffffff")
	dc.DrawRoundedRectangle(left, top, float64(innerWidth), height, 22)
	dc.Fill()
	dc.SetHexColor("#cbd5e1")
	dc.DrawRoundedRectangle(left, top, 8, height, 4)
	dc.Fill()

	title = strings.TrimSpace(title)
	if title == "" {
		title = "没有课的群友"
	}
	fc.drawText(dc, left+foldedPadX, top+foldedPadY, fmt.Sprintf("%s · %d 人", title, len(rows)), 19, "#475569", true, 0)

	columns := foldedColumns(innerWidth)
	gridTop := top + foldedPadY + foldedTitleH
	for index, row := range rows {
		column := index % columns
		line := index / columns
		cellLeft := left + foldedPadX + float64(column*foldedCellWidth)
		cellTop := gridTop + float64(line*foldedRowHeight)
		dc.DrawImage(fc.memberAvatar(row, avatars, foldedAvatarSize), int(cellLeft), int(cellTop))
		fc.drawText(dc, cellLeft+foldedAvatarSize+12, cellTop+11, displayName(row), 19, "#64748b", false, foldedNameWidth)
	}
}

func displayName(row schedule.DayRow) string {
	if strings.TrimSpace(row.Name) != "" {
		return row.Name
	}
	return row.UserID
}

// memberAvatar prefers a real avatar and falls back to the initial circle.
func (fc *faceCache) memberAvatar(row schedule.DayRow, avatars map[string]image.Image, size int) image.Image {
	if avatars != nil {
		if avatar, ok := avatars[row.UserID]; ok && avatar != nil {
			return circleImage(avatar, size)
		}
	}
	return fc.initialAvatar(row.Name, row.UserID, size)
}

// initialAvatar draws a deterministic colored circle with the first grapheme.
func (fc *faceCache) initialAvatar(name, userID string, size int) image.Image {
	rgba := image.NewRGBA(image.Rect(0, 0, size, size))
	dc := gg.NewContextForRGBA(rgba)
	dc.DrawCircle(float64(size)/2, float64(size)/2, float64(size)/2)
	dc.SetHexColor(paletteColor(userID))
	dc.Fill()
	initial := firstGrapheme(name)
	if initial == "" {
		initial = "?"
	}
	fontSize := int(float64(size) * 0.5)
	width := fc.measureText(initial, fontSize, true)
	fc.drawText(dc, float64(size)/2-width/2, float64(size)*0.22, initial, fontSize, "#ffffff", true, 0)
	return rgba
}

var avatarPalette = []string{
	"#4f6bed", "#0ea5e9", "#14b8a6", "#f59e0b",
	"#ef4444", "#8b5cf6", "#10b981", "#f97316",
}

func paletteColor(userID string) string {
	if userID == "" {
		return avatarPalette[0]
	}
	hash := uint32(2166136261)
	for i := 0; i < len(userID); i++ {
		hash ^= uint32(userID[i])
		hash *= 16777619
	}
	// Final avalanche mix, then drop the low bits: FNV modulo a small number
	// clusters badly for ids that share the same characters.
	hash ^= hash >> 16
	hash *= 2246822507
	hash ^= hash >> 13
	return avatarPalette[hash%uint32(len(avatarPalette))]
}

func firstGrapheme(text string) string {
	clusters := graphemes(strings.TrimSpace(text))
	if len(clusters) == 0 {
		return ""
	}
	return clusters[0]
}

// circleImage scales src to cover size x size (center-cropped) and masks it
// into a circle. Scaling is required: DrawImage alone would paste the source at
// its native resolution, showing only the middle of a large avatar.
func circleImage(src image.Image, size int) *image.RGBA {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	masked := image.NewRGBA(image.Rect(0, 0, size, size))
	if width <= 0 || height <= 0 {
		return masked
	}
	scale := math.Max(float64(size)/float64(width), float64(size)/float64(height))
	stageWidth := int(math.Ceil(float64(width) * scale))
	stageHeight := int(math.Ceil(float64(height) * scale))
	stage := image.NewRGBA(image.Rect(0, 0, stageWidth, stageHeight))
	xdraw.CatmullRom.Scale(stage, stage.Bounds(), src, bounds, xdraw.Src, nil)
	scaled := image.NewRGBA(image.Rect(0, 0, size, size))
	offset := image.Point{X: (stageWidth - size) / 2, Y: (stageHeight - size) / 2}
	xdraw.Draw(scaled, scaled.Bounds(), stage, offset, xdraw.Src)

	dc := gg.NewContextForRGBA(masked)
	dc.DrawCircle(float64(size)/2, float64(size)/2, float64(size)/2)
	dc.Clip()
	dc.DrawImage(scaled, 0, 0)
	return masked
}

// sortRowsForTest keeps deterministic output when callers pass unsorted rows.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func sortRowsForTest(rows []schedule.DayRow) []schedule.DayRow {
	sorted := append([]schedule.DayRow(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].UserID < sorted[j].UserID })
	return sorted
}
