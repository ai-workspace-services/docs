package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func contentCacheHandler(hash string) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	return WithContentCache(func() string { return hash }, time.Minute, inner)
}

func TestContentCacheAnnotatesReads(t *testing.T) {
	recorder := httptest.NewRecorder()
	contentCacheHandler("abc123").ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/blogs?lang=zh", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := recorder.Header().Get("ETag"); got == "" {
		t.Fatal("expected an ETag")
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=0, s-maxage=60, stale-while-revalidate=120" {
		t.Fatalf("unexpected Cache-Control: %q", got)
	}
	if got := recorder.Header().Get("X-Content-Hash"); got != "abc123" {
		t.Fatalf("unexpected X-Content-Hash: %q", got)
	}
}

func TestContentCacheReturnsNotModifiedForKnownETag(t *testing.T) {
	handler := contentCacheHandler("abc123")

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/blogs?lang=zh", nil))
	etag := first.Header().Get("ETag")

	request := httptest.NewRequest(http.MethodGet, "/api/v1/blogs?lang=zh", nil)
	request.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, request)

	if second.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Fatalf("expected an empty body, got %q", second.Body.String())
	}
}

func TestContentCacheETagTracksContentAndLanguage(t *testing.T) {
	zh := httptest.NewRecorder()
	contentCacheHandler("abc123").ServeHTTP(zh, httptest.NewRequest(http.MethodGet, "/api/v1/blogs?lang=zh", nil))
	en := httptest.NewRecorder()
	contentCacheHandler("abc123").ServeHTTP(en, httptest.NewRequest(http.MethodGet, "/api/v1/blogs?lang=en", nil))
	released := httptest.NewRecorder()
	contentCacheHandler("def456").ServeHTTP(released, httptest.NewRequest(http.MethodGet, "/api/v1/blogs?lang=zh", nil))

	if zh.Header().Get("ETag") == en.Header().Get("ETag") {
		t.Fatal("expected a different ETag per language")
	}
	if zh.Header().Get("ETag") == released.Header().Get("ETag") {
		t.Fatal("expected a different ETag after a content release")
	}
}

func TestContentCacheResolvesLanguageFromHeaderWithoutQuery(t *testing.T) {
	handler := contentCacheHandler("abc123")

	zhRequest := httptest.NewRequest(http.MethodGet, "/api/v1/blogs", nil)
	zhRequest.Header.Set("Accept-Language", "zh-CN")
	zh := httptest.NewRecorder()
	handler.ServeHTTP(zh, zhRequest)

	enRequest := httptest.NewRequest(http.MethodGet, "/api/v1/blogs", nil)
	enRequest.Header.Set("Accept-Language", "en-US")
	en := httptest.NewRecorder()
	handler.ServeHTTP(en, enRequest)

	if zh.Header().Get("ETag") == en.Header().Get("ETag") {
		t.Fatal("expected the negotiated language to change the ETag")
	}
	if got := zh.Header().Get("Vary"); got != "Accept-Language, X-Language" {
		t.Fatalf("unexpected Vary: %q", got)
	}
}

func TestContentCacheLeavesWritesAlone(t *testing.T) {
	recorder := httptest.NewRecorder()
	contentCacheHandler("abc123").ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/admin/reload", nil))

	if recorder.Header().Get("ETag") != "" {
		t.Fatal("a write must not be advertised as cacheable")
	}
	if recorder.Header().Get("Cache-Control") != "" {
		t.Fatal("a write must not carry a Cache-Control window")
	}
}
