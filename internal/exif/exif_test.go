package exif

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
)

func TestExtractNoEXIFJPEGDoesNotFail(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	result, err := Extract(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if result.Found {
		t.Fatalf("expected no EXIF metadata in generated JPEG")
	}
	if result.Metadata["exif_available"] != false {
		t.Fatalf("expected exif_available=false, got %#v", result.Metadata)
	}
}

func TestExtractMalformedInputDoesNotFail(t *testing.T) {
	result, err := Extract(strings.NewReader("not a jpeg"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Found {
		t.Fatalf("expected malformed input to be treated as no usable EXIF")
	}
	if result.Metadata["exif_error"] == "" {
		t.Fatalf("expected parser error to be recorded")
	}
}
