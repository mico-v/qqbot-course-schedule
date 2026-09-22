package render

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func sampleRows() []schedule.DayRow {
	return []schedule.DayRow{
		{
			UserID: "U1", Name: "小明", StatusKey: "active", Status: "正在上课",
			Course: "高等数学", Location: "教一101", TimeText: "09:00 - 10:30",
			Duration: "1小时30分钟", CountdownLabel: "距下课", Countdown: "45分钟",
			Progress: 0.5, CourseCount: 1,
		},
		{
			UserID: "U2", Name: "小红", StatusKey: "upcoming", Status: "下一节即将上课",
			Course: "大学英语", TimeText: "11:00 - 12:00", Duration: "1小时",
			CountdownLabel: "距上课", Countdown: "1小时30分钟", CourseCount: 2,
		},
	}
}

func TestDayCardRendersExpectedSize(t *testing.T) {
	renderer, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	image := renderer.DayCard(DayCardData{
		Title:       "课程表 · 2026-09-17 周四",
		Footer:      "实时状态 · 课程时间以本地时区为准",
		FoldedTitle: "今天已经没有课的群友",
		Rows:        sampleRows(),
		Folded: []schedule.DayRow{
			{UserID: "U3", Name: "小刚", StatusKey: "none", Status: "今日无课"},
		},
	})
	if got := image.Bounds().Dx(); got != cardWidth {
		t.Errorf("width = %d, want %d", got, cardWidth)
	}
	if got := image.Bounds().Dy(); got < 360 {
		t.Errorf("height = %d, want >= 360", got)
	}
}

func TestDayCardEmptyScope(t *testing.T) {
	renderer, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	image := renderer.DayCard(DayCardData{Title: "课程表", Footer: "实时状态"})
	if image.Bounds().Dx() != cardWidth || image.Bounds().Dy() < 360 {
		t.Errorf("empty card size = %v", image.Bounds())
	}
}

func TestSaveJPEG(t *testing.T) {
	renderer, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	image := renderer.DayCard(DayCardData{Title: "课程表", Footer: "实时状态", Rows: sampleRows()})
	dir := t.TempDir()
	name, err := SaveJPEG(image, dir, "schedule_20260917")
	if err != nil {
		t.Fatalf("SaveJPEG: %v", err)
	}
	if !strings.HasPrefix(name, "schedule_20260917_") || !strings.HasSuffix(name, ".jpg") {
		t.Errorf("name = %q", name)
	}
	info, err := os.Stat(filepath.Join(dir, name))
	if err != nil || info.Size() == 0 {
		t.Fatalf("saved file missing: %v", err)
	}
}

func TestInitialAvatarIsDeterministic(t *testing.T) {
	fonts, err := LoadFonts()
	if err != nil {
		t.Fatalf("LoadFonts: %v", err)
	}
	fc := newFaceCache(fonts)
	first := fc.initialAvatar("小明", "U1", 76)
	second := fc.initialAvatar("小明", "U1", 76)
	if first.Bounds() != second.Bounds() {
		t.Fatal("avatar bounds differ")
	}
	if paletteColor("U1") != paletteColor("U1") || paletteColor("U1") == paletteColor("U2") && paletteColor("U1") == "" {
		t.Fatal("palette must be deterministic")
	}
}

func TestDayCardUsesRealAvatars(t *testing.T) {
	renderer, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	avatar := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			avatar.Set(x, y, color.RGBA{20, 180, 90, 255})
		}
	}
	rendered := renderer.DayCard(DayCardData{
		Title:   "课程表",
		Footer:  "footer",
		Rows:    sampleRows()[:1],
		Avatars: map[string]image.Image{"U1": avatar},
	})
	r, g, b, _ := rendered.At(62+38, headerHeight+40+38).RGBA()
	if g>>8 < 120 || r>>8 > 120 || b>>8 > 120 {
		t.Fatalf("avatar pixel = %d/%d/%d, want the green real avatar", r>>8, g>>8, b>>8)
	}
}
