package render

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// avatarPattern builds a size x size image with a red border, a green center
// block and a blue field, so scaling mistakes are easy to spot.
func avatarPattern(size int) *image.RGBA {
	src := image.NewRGBA(image.Rect(0, 0, size, size))
	border := size / 16
	centerFrom := size * 5 / 16
	centerTo := size * 11 / 16
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			switch {
			case x < border || x >= size-border || y < border || y >= size-border:
				src.Set(x, y, color.RGBA{220, 40, 40, 255})
			case x >= centerFrom && x < centerTo && y >= centerFrom && y < centerTo:
				src.Set(x, y, color.RGBA{30, 180, 80, 255})
			default:
				src.Set(x, y, color.RGBA{40, 80, 220, 255})
			}
		}
	}
	return src
}

func TestCircleImageScalesSource(t *testing.T) {
	out := circleImage(avatarPattern(640), 88)
	if out.Bounds().Dx() != 88 || out.Bounds().Dy() != 88 {
		t.Fatalf("size = %v", out.Bounds())
	}
	// The top edge must come from the source border; the old bug showed the
	// center block there because the source was pasted at native size.
	if r, g, b, _ := out.At(44, 2).RGBA(); r>>8 < 150 || g>>8 > 120 || b>>8 > 120 {
		t.Fatalf("top edge = %d/%d/%d, want the red source border", r>>8, g>>8, b>>8)
	}
	if r, g, b, _ := out.At(44, 44).RGBA(); g>>8 < 120 || r>>8 > 120 {
		t.Fatalf("center = %d/%d/%d, want green", r>>8, g>>8, b>>8)
	}
	if _, _, _, a := out.At(1, 1).RGBA(); a != 0 {
		t.Fatalf("corner alpha = %d, want transparent outside the circle", a)
	}
}

func TestCircleImageHandlesNonSquareSource(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 640, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 640; x++ {
			src.Set(x, y, color.RGBA{200, 200, 40, 255})
		}
	}
	out := circleImage(src, 88)
	if r, g, b, _ := out.At(44, 44).RGBA(); r>>8 < 150 || g>>8 < 150 || b>>8 > 120 {
		t.Fatalf("center = %d/%d/%d, want the source colour", r>>8, g>>8, b>>8)
	}
}

func TestLoadCachedBotAvatar(t *testing.T) {
	dataDir := t.TempDir()
	if avatar := LoadCachedBotAvatar(dataDir); avatar != nil {
		t.Fatalf("missing cache should return nil, got %v", avatar.Bounds())
	}
	path := filepath.Join(dataDir, "avatars", "bot.png")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, avatarPattern(256)); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
	avatar := LoadCachedBotAvatar(dataDir)
	if avatar == nil {
		t.Fatal("cached avatar should load")
	}
	if avatar.Bounds().Dx() != botAvatarSize || avatar.Bounds().Dy() != botAvatarSize {
		t.Fatalf("size = %v, want %d", avatar.Bounds(), botAvatarSize)
	}
}
