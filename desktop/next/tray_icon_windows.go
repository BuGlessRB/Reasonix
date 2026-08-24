//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
)

// Windows reads the notification area's icon as an .ico. Since Vista an entry
// may hold a PNG verbatim, so the container is a 22-byte header around the
// bytes we already have rather than a second encoder.
func encodeIcon(img image.Image) []byte {
	var body bytes.Buffer
	if err := png.Encode(&body, img); err != nil {
		return nil
	}
	bounds := img.Bounds()
	var out bytes.Buffer
	// ICONDIR: reserved, type 1 (icon), one image.
	_ = binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, 1})
	// ICONDIRENTRY: 0 in a size byte means 256, which is why a 64px mark fits.
	out.Write([]byte{byte(bounds.Dx()), byte(bounds.Dy()), 0, 0})
	_ = binary.Write(&out, binary.LittleEndian, [2]uint16{1, 32})
	_ = binary.Write(&out, binary.LittleEndian, uint32(body.Len()))
	_ = binary.Write(&out, binary.LittleEndian, uint32(22))
	out.Write(body.Bytes())
	return out.Bytes()
}
