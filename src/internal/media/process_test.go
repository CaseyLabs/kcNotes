package media

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestNormalizePNGAndBuildVariants(t *testing.T) {
	t.Parallel()

	img := image.NewRGBA(image.Rect(0, 0, 640, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 640; x++ {
			img.Set(x, y, color.RGBA{R: 0x33, G: 0x66, B: 0x99, A: 0xff})
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, img); err != nil {
		t.Fatalf("encode input: %v", err)
	}

	normalized, err := Normalize(input.Bytes(), "image/png")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if normalized.Width != 640 || normalized.Height != 320 {
		t.Fatalf("expected original dimensions, got %dx%d", normalized.Width, normalized.Height)
	}

	variants, err := BuildVariants(normalized, []VariantSpec{{Name: "thumb", MaxSide: 320}})
	if err != nil {
		t.Fatalf("build variants: %v", err)
	}
	if len(variants) != 1 {
		t.Fatalf("expected one variant, got %d", len(variants))
	}
	if variants[0].Width != 320 || variants[0].Height != 160 {
		t.Fatalf("expected 320x160 variant, got %dx%d", variants[0].Width, variants[0].Height)
	}
}

func TestNormalizeRejectsOversizedDimensionsBeforeDecode(t *testing.T) {
	t.Parallel()

	_, err := Normalize(pngHeaderWithDimensions(maxImageDimension+1, 1), "image/png")
	if err == nil {
		t.Fatalf("expected oversized image rejection")
	}
	if !strings.Contains(err.Error(), "dimensions exceed limit") {
		t.Fatalf("expected dimension limit error, got %v", err)
	}
}

func pngHeaderWithDimensions(width, height uint32) []byte {
	var data bytes.Buffer
	data.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8
	ihdr[9] = 2

	writePNGChunk(&data, "IHDR", ihdr)
	writePNGChunk(&data, "IEND", nil)
	return data.Bytes()
}

func writePNGChunk(dst *bytes.Buffer, kind string, payload []byte) {
	_ = binary.Write(dst, binary.BigEndian, uint32(len(payload)))
	dst.WriteString(kind)
	dst.Write(payload)
	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte(kind))
	_, _ = crc.Write(payload)
	_ = binary.Write(dst, binary.BigEndian, crc.Sum32())
}
