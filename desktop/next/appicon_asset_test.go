package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"testing"
)

// macOS reserves a margin around a dock icon; artwork that fills the frame
// renders visibly larger than every system icon beside it.
func TestDarwinICNSUsesMacOSIconSafeArea(t *testing.T) {
	img := decodeICNSImage(t, "appicon.icns", "ic10")
	if got, want := img.Bounds(), image.Rect(0, 0, 1024, 1024); got != want {
		t.Fatalf("macOS ic10 frame bounds = %v, want %v", got, want)
	}
	if got, want := alphaBounds(img), image.Rect(100, 100, 924, 924); got != want {
		t.Fatalf("macOS icon visible bounds = %v, want %v", got, want)
	}
}

func alphaBounds(img image.Image) image.Rectangle {
	bounds := img.Bounds()
	visible := image.Rectangle{}
	found := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			if !found {
				visible = image.Rect(x, y, x+1, y+1)
				found = true
				continue
			}
			if x < visible.Min.X {
				visible.Min.X = x
			}
			if y < visible.Min.Y {
				visible.Min.Y = y
			}
			if x >= visible.Max.X {
				visible.Max.X = x + 1
			}
			if y >= visible.Max.Y {
				visible.Max.Y = y + 1
			}
		}
	}
	return visible
}

func decodeICNSImage(t *testing.T, path, iconType string) image.Image {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 8 || string(data[:4]) != "icns" {
		t.Fatal("invalid ICNS header")
	}
	if declaredSize := int(binary.BigEndian.Uint32(data[4:8])); declaredSize != len(data) {
		t.Fatalf("ICNS size = %d, want %d", declaredSize, len(data))
	}

	for offset := 8; offset < len(data); {
		if offset+8 > len(data) {
			t.Fatalf("truncated ICNS entry header at offset %d", offset)
		}
		entryType := string(data[offset : offset+4])
		entrySize := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		if entrySize < 8 || offset+entrySize > len(data) {
			t.Fatalf("invalid ICNS entry %q size %d at offset %d", entryType, entrySize, offset)
		}
		if entryType == iconType {
			img, err := png.Decode(bytes.NewReader(data[offset+8 : offset+entrySize]))
			if err != nil {
				t.Fatalf("decode ICNS entry %q: %v", iconType, err)
			}
			return img
		}
		offset += entrySize
	}

	t.Fatalf("ICNS is missing %q image", iconType)
	return nil
}
