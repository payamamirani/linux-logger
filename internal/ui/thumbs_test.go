package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// makeJPEG renders a w×h test image and encodes it as JPEG.
func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestThumbnailScalesToHeight(t *testing.T) {
	for _, src := range [][2]int{{1920, 1080}, {640, 480}, {200, 400}} {
		jpg := makeJPEG(t, src[0], src[1])
		pngBytes, err := thumbnail(jpg, 96)
		if err != nil {
			t.Fatalf("thumbnail(%dx%d): %v", src[0], src[1], err)
		}

		// Output must be a valid PNG with exactly 96px height and the
		// source aspect ratio.
		img, err := png.Decode(bytes.NewReader(pngBytes))
		if err != nil {
			t.Fatalf("decode thumbnail: %v", err)
		}
		b := img.Bounds()
		if b.Dy() != 96 {
			t.Errorf("%dx%d: height = %d, want 96", src[0], src[1], b.Dy())
		}
		wantW := src[0] * 96 / src[1]
		if b.Dx() != wantW {
			t.Errorf("%dx%d: width = %d, want %d", src[0], src[1], b.Dx(), wantW)
		}
	}
}

func TestThumbnailRejectsGarbage(t *testing.T) {
	if _, err := thumbnail([]byte("not an image"), 96); err == nil {
		t.Fatal("expected error for garbage input")
	}
}
