package operations

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCachePublicServesBodyWithMetadata(t *testing.T) {
	handler := cachePublic(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/public/x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheableControl {
		t.Fatalf("Cache-Control = %q, want %q", got, cacheableControl)
	}
	etag := rec.Header().Get("ETag")
	if len(etag) != 66 || !strings.HasPrefix(etag, `"`) {
		t.Fatalf("unexpected ETag %q", etag)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestCachePublicAnswers304OnRevalidation(t *testing.T) {
	var etag string
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	first := httptest.NewRecorder()
	cachePublic(inner).ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	etag = first.Header().Get("ETag")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	cachePublic(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("304 must have no body, got %q", rec.Body.String())
	}
}

func TestCachePublicRecomputesAfterContentChange(t *testing.T) {
	value := "a"
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(value))
	})
	first := httptest.NewRecorder()
	cachePublic(inner).ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", first.Header().Get("ETag"))
	value = "b"
	rec := httptest.NewRecorder()
	cachePublic(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after content change", rec.Code)
	}
	if rec.Body.String() != "b" {
		t.Fatalf("body = %q, want changed content", rec.Body.String())
	}
}

func TestCachePublicLeavesErrorsUncacheable(t *testing.T) {
	handler := cachePublic(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got == cacheableControl {
		t.Fatalf("errors must not be cacheable, got %q", got)
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Fatalf("errors must not have ETag, got %q", got)
	}
}

func TestCachePublicStreamsOversizedResponses(t *testing.T) {
	big := strings.Repeat("x", maxBufferedBody+1)
	handler := cachePublic(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(big))
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != big {
		t.Fatalf("body must stream intact")
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Fatalf("oversized responses must skip ETag, got %q", got)
	}
}

func TestCachePublicIgnoresNonGETAndRangeRequests(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Called", r.Method)
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	cachePublic(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Header().Get("Cache-Control") == cacheableControl {
		t.Fatal("non-GET must pass through untouched")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Range", "bytes=0-1")
	rec = httptest.NewRecorder()
	cachePublic(inner).ServeHTTP(rec, req)
	if rec.Header().Get("Cache-Control") == cacheableControl {
		t.Fatal("range requests must pass through untouched")
	}
}
