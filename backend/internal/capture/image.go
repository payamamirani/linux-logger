package capture

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"os"
	"time"
)

// pngFileToJPEG reads a PNG file, re-encodes it as JPEG in memory and
// removes nothing (the caller owns the source file).
func pngFileToJPEG(path string, quality int) (*Screenshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open png: %w", err)
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	if format != "png" {
		return nil, fmt.Errorf("unexpected image format %q (want png)", format)
	}

	b := img.Bounds()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}

	return &Screenshot{
		Data:    buf.Bytes(),
		MIME:    "image/jpeg",
		TakenAt: time.Now(),
		Width:   b.Dx(),
		Height:  b.Dy(),
	}, nil
}
