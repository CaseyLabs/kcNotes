package media

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
)

const (
	maxImageDimension = 10_000
	maxImagePixels    = 40_000_000
)

type ProcessedImage struct {
	Data   []byte
	MIME   string
	Width  int
	Height int
}

type VariantSpec struct {
	Name    string
	MaxSide int
}

type Variant struct {
	Name string
	ProcessedImage
}

func Normalize(data []byte, mimeType string) (ProcessedImage, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ProcessedImage{}, fmt.Errorf("decode image dimensions: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return ProcessedImage{}, fmt.Errorf("invalid image dimensions")
	}
	if cfg.Width > maxImageDimension || cfg.Height > maxImageDimension || cfg.Width > maxImagePixels/cfg.Height {
		return ProcessedImage{}, fmt.Errorf("image dimensions exceed limit")
	}

	switch mimeType {
	case "image/jpeg", "image/png":
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return ProcessedImage{}, fmt.Errorf("decode image: %w", err)
		}
		normalized, err := encodeImage(img, mimeType)
		if err != nil {
			return ProcessedImage{}, err
		}
		return ProcessedImage{Data: normalized, MIME: mimeType, Width: cfg.Width, Height: cfg.Height}, nil
	case "image/gif":
		if _, err := gif.DecodeConfig(bytes.NewReader(data)); err != nil {
			return ProcessedImage{}, fmt.Errorf("decode gif dimensions: %w", err)
		}
		return ProcessedImage{Data: data, MIME: mimeType, Width: cfg.Width, Height: cfg.Height}, nil
	default:
		return ProcessedImage{}, fmt.Errorf("unsupported image type")
	}
}

func BuildVariants(original ProcessedImage, specs []VariantSpec) ([]Variant, error) {
	if original.MIME == "image/gif" {
		return nil, nil
	}
	src, _, err := image.Decode(bytes.NewReader(original.Data))
	if err != nil {
		return nil, fmt.Errorf("decode normalized image: %w", err)
	}

	variants := make([]Variant, 0, len(specs))
	for _, spec := range specs {
		if spec.Name == "" || spec.MaxSide <= 0 {
			continue
		}
		width, height := boundedSize(original.Width, original.Height, spec.MaxSide)
		if width == original.Width && height == original.Height {
			continue
		}
		resized := resizeNearest(src, width, height)
		data, err := encodeImage(resized, original.MIME)
		if err != nil {
			return nil, err
		}
		variants = append(variants, Variant{
			Name: spec.Name,
			ProcessedImage: ProcessedImage{
				Data:   data,
				MIME:   original.MIME,
				Width:  width,
				Height: height,
			},
		})
	}
	return variants, nil
}

func encodeImage(img image.Image, mimeType string) ([]byte, error) {
	var out bytes.Buffer
	switch mimeType {
	case "image/jpeg":
		if err := jpeg.Encode(&out, flattenAlpha(img), &jpeg.Options{Quality: 85}); err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
	case "image/png":
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(&out, img); err != nil {
			return nil, fmt.Errorf("encode png: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported image type")
	}
	return out.Bytes(), nil
}

func flattenAlpha(img image.Image) image.Image {
	bounds := img.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, bounds, img, bounds.Min, draw.Over)
	return dst
}

func boundedSize(width, height, maxSide int) (int, int) {
	longest := max(width, height)
	if longest <= maxSide {
		return width, height
	}
	scale := float64(maxSide) / float64(longest)
	return max(1, int(math.Round(float64(width)*scale))), max(1, int(math.Round(float64(height)*scale)))
}

func resizeNearest(src image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	sb := src.Bounds()
	for y := 0; y < height; y++ {
		sy := sb.Min.Y + y*sb.Dy()/height
		for x := 0; x < width; x++ {
			sx := sb.Min.X + x*sb.Dx()/width
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func DecodeConfig(r io.Reader) (image.Config, string, error) {
	return image.DecodeConfig(r)
}
