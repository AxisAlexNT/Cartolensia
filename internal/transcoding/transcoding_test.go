package transcoding

import "testing"

func TestParseEncoders(t *testing.T) {
	const sample = `
Encoders:
 V..... = Video
 V....D libx264              libx264 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10
 V....D h264_nvenc           NVIDIA NVENC H.264 encoder
 V....D hevc_vaapi           H.265/HEVC VAAPI encoder
 V....D av1_qsv              AV1 Intel Quick Sync Video acceleration
 A..... aac                  AAC audio
`
	encoders := ParseEncoders(sample)
	if len(encoders) != 4 {
		t.Fatalf("expected four video encoders, got %#v", encoders)
	}
	var foundNVENC, foundVAAPI, foundAV1 bool
	for _, encoder := range encoders {
		if encoder.Name == "h264_nvenc" && encoder.Hardware == "nvidia_nvenc" && encoder.CodecFamily == "h264" {
			foundNVENC = true
		}
		if encoder.Name == "hevc_vaapi" && encoder.Hardware == "vaapi" && encoder.CodecFamily == "hevc" {
			foundVAAPI = true
		}
		if encoder.Name == "av1_qsv" && encoder.Hardware == "intel_qsv" && encoder.CodecFamily == "av1" {
			foundAV1 = true
		}
	}
	if !foundNVENC || !foundVAAPI || !foundAV1 {
		t.Fatalf("missing expected parsed encoders: %#v", encoders)
	}
}
