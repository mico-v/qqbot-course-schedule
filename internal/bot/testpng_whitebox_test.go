package bot

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// testPNG builds a small PNG used by avatar tests.
func testPNG(t interface{ Fatal(args ...any) }) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{20, 160, 90, 255})
		}
	}
	buffer := &bytes.Buffer{}
	if err := png.Encode(buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
