//go:build !windows

package main

import (
	"bytes"
	"image"
	"image/png"
)

// macOS and the freedesktop status hosts both take a PNG.
func encodeIcon(img image.Image) []byte {
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil
	}
	return out.Bytes()
}
