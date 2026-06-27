package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPSRedirectHandlerUsesConfiguredTLSPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://192.0.2.10:18080/?page=explorer", nil)
	rec := httptest.NewRecorder()

	httpsRedirectHandler(":18443", nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected temporary redirect, got %d", rec.Code)
	}
	if got, want := rec.Header().Get("Location"), "https://192.0.2.10:18443/?page=explorer"; got != want {
		t.Fatalf("unexpected redirect location %q, want %q", got, want)
	}
}

func TestHTTPSRedirectHandlerBypassesAIMedia(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:18080/api/v1/ai-media/asset/original?token=t", nil)
	rec := httptest.NewRecorder()
	bypass := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	httpsRedirectHandler(":18443", bypass).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected AI media bypass, got %d", rec.Code)
	}
}
