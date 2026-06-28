package server

import (
	"testing"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
)

func TestAISupportsAssetUsesDecodableExtensions(t *testing.T) {
	jpg := catalog.Asset{MediaKind: "photo", Locations: []catalog.Location{{Extension: "jpg"}}}
	svg := catalog.Asset{MediaKind: "photo", Locations: []catalog.Location{{Extension: "svg"}}}
	mp3 := catalog.Asset{MediaKind: "audio", Locations: []catalog.Location{{Extension: "mp3"}}}
	playlist := catalog.Asset{MediaKind: "audio", Locations: []catalog.Location{{Extension: "pls"}}}
	video := catalog.Asset{MediaKind: "video", Locations: []catalog.Location{{Extension: "mp4"}}}
	document := catalog.Asset{MediaKind: "document", Locations: []catalog.Location{{Extension: "pdf"}}}

	if !aiSupportsAsset("describe_image", jpg) {
		t.Fatal("jpg photo should be eligible for image description")
	}
	if aiSupportsAsset("describe_image", svg) {
		t.Fatal("svg photo-like asset should not be sent to raster image models")
	}
	if !aiSupportsAsset("transcribe_audio", mp3) {
		t.Fatal("mp3 audio should be eligible for transcription")
	}
	if aiSupportsAsset("transcribe_audio", playlist) {
		t.Fatal("playlist metadata should not be sent to ASR")
	}
	if !aiSupportsAsset("transcribe_audio", video) {
		t.Fatal("mp4 video should be eligible for audio transcription")
	}
	if aiSupportsAsset("ocr_image", document) {
		t.Fatal("PDF document OCR uses the document pipeline, not image OCR")
	}
}
