package media

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
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
