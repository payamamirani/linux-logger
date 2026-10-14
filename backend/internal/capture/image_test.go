package capture

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestPNGFileToJPEG(t *testing.T) {
	// Build a small test PNG in memory.
	const w, h = 64, 32
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 8), B: 128, A: 255})
		}
	}

	path := filepath.Join(t.TempDir(), "test.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	shot, err := pngFileToJPEG(path, 80)
	if err != nil {
		t.Fatalf("pngFileToJPEG: %v", err)
	}

	if shot.MIME != "image/jpeg" {
		t.Errorf("MIME = %q, want image/jpeg", shot.MIME)
	}
	if shot.Width != w || shot.Height != h {
		t.Errorf("dimensions = %dx%d, want %dx%d", shot.Width, shot.Height, w, h)
	}
	if shot.TakenAt.IsZero() {
		t.Error("TakenAt is zero")
	}

	// Data must decode as a valid JPEG.
	dec, format, err := image.Decode(bytes.NewReader(shot.Data))
	if err != nil {
		t.Fatalf("decode jpeg: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("decoded format = %q, want jpeg", format)
	}
	if got := dec.Bounds().Dx(); got != w {
		t.Errorf("decoded width = %d, want %d", got, w)
	}
}

func TestPNGFileToJPEGRejectsNonPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake.png")
	if err := os.WriteFile(path, []byte("not a png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := pngFileToJPEG(path, 80); err == nil {
		t.Fatal("expected error for non-PNG input, got nil")
	}
}

// Compile-time check that jpeg stays registered (guards the blank import in
// image.go being accidentally repurposed).
var _ = jpeg.DefaultQuality
