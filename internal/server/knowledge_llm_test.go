package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestLocalLLMURLHandlesV1Endpoint(t *testing.T) {
	got := localLLMURL("http://127.0.0.1:8000/v1", "/v1/chat/completions")
	if got != "http://127.0.0.1:8000/v1/chat/completions" {
		t.Fatalf("unexpected URL: %s", got)
	}
	got = localLLMURL("http://127.0.0.1:11434", "/api/chat")
	if got != "http://127.0.0.1:11434/api/chat" {
		t.Fatalf("unexpected Ollama URL: %s", got)
	}
}

func TestParseKnowledgeToolRequestsRejectsUnknownTools(t *testing.T) {
	requests, err := parseKnowledgeToolRequests(`{
		"tools": [
			{"tool":"search_media","query":"kind:photo 2025-05..2025-08 train","limit": 12},
			{"tool":"delete_assets","query":"all"},
			{"tool":"readonly_sql","sql":"select asset_id from cartolensia_search_assets","limit": 2000},
			{"tool":"transcode_recommendations","query":"kind:video hevc","limit": 8},
			{"tool":"find_segmented_video_series","query":"thm mp4 series","limit": 8}
		]
	}`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(requests) != 4 {
		t.Fatalf("expected four safe requests, got %#v", requests)
	}
	if requests[1].Tool != "readonly_sql" || requests[1].Limit != 50 {
		t.Fatalf("readonly SQL request was not capped: %#v", requests[1])
	}
	if requests[2].Tool != "transcode_recommendations" || requests[3].Tool != "find_segmented_video_series" {
		t.Fatalf("expected safe action tools to pass allowlist: %#v", requests)
	}
}

func TestPostOllamaChat(t *testing.T) {
	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"message":{"content":"local answer"}}`)),
		}, nil
	})}
	answer, err := postOllamaChat(context.Background(), "http://llm.local", "fixture", "question")
	if err != nil {
		t.Fatalf("postOllamaChat failed: %v", err)
	}
	if answer != "local answer" {
		t.Fatalf("unexpected answer %q", answer)
	}
}

func TestPostOpenAICompatibleChatWithV1Base(t *testing.T) {
	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"choices":[{"message":{"content":"vllm answer"}}]}`)),
		}, nil
	})}
	answer, err := postOpenAICompatibleChat(context.Background(), "http://llm.local/v1", "fixture", "question")
	if err != nil {
		t.Fatalf("postOpenAICompatibleChat failed: %v", err)
	}
	if answer != "vllm answer" {
		t.Fatalf("unexpected answer %q", answer)
	}
}

func TestSearchPlanRussianMonthRange(t *testing.T) {
	plan := (&Server{}).buildSearchPlan("kind:photo фотографии май-август 2025 года поездками пожалуйста")
	joined := strings.Join(plan.Tokens, " ")
	if !strings.Contains(joined, "2025-05..2025-08") {
		t.Fatalf("expected date range token, got %#v", plan.Tokens)
	}
	if strings.Contains(joined, "май-август") {
		t.Fatalf("month words should not remain as ordinary tokens: %#v", plan.Tokens)
	}
}

func TestSegmentedSeriesCandidatesGroupsSequentialVideoParts(t *testing.T) {
	rows := []map[string]any{
		{"asset_id": "thumb-a", "display_name": "CAM_0001.thm", "storage_name": "originals", "relative_path": "DCIM/CAM_0001.thm", "file_name": "CAM_0001.thm", "extension": "thm"},
		{"asset_id": "video-a", "display_name": "CAM_0001.mp4", "storage_name": "originals", "relative_path": "DCIM/CAM_0001.mp4", "file_name": "CAM_0001.mp4", "extension": "mp4", "size_bytes": int64(10)},
		{"asset_id": "video-b", "display_name": "CAM_0002.mp4", "storage_name": "originals", "relative_path": "DCIM/CAM_0002.mp4", "file_name": "CAM_0002.mp4", "extension": "mp4", "size_bytes": int64(20)},
		{"asset_id": "other", "display_name": "IMG_0001.jpg", "storage_name": "originals", "relative_path": "DCIM/IMG_0001.jpg", "file_name": "IMG_0001.jpg", "extension": "jpg"},
	}
	candidates := segmentedSeriesCandidates(rows)
	if len(candidates) != 1 {
		t.Fatalf("expected one segmented candidate, got %#v", candidates)
	}
	if candidates[0].Prefix != "CAM" || len(candidates[0].Segments) != 2 || len(candidates[0].Delimiters) != 1 {
		t.Fatalf("unexpected segmented candidate: %#v", candidates[0])
	}
	if candidates[0].Segments[0].AssetID != "video-a" || candidates[0].Segments[1].AssetID != "video-b" {
		t.Fatalf("segments are not sorted by numeric part: %#v", candidates[0].Segments)
	}
}
