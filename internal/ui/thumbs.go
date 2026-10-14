package ui

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
)

// thumbnail decodes a JPEG and returns a height-scaled thumbnail as PNG
// bytes (PNG because gdk.Texture.NewFromBytes sniffs encoded data).
func thumbnail(jpegBytes []byte, height int) ([]byte, error) {
	src, err := jpeg.Decode(bytes.NewReader(jpegBytes))
	if err != nil {
		return nil, err
	}

	b := src.Bounds()
	width := b.Dx() * height / b.Dy()
	if width < 1 {
		width = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
