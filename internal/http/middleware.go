package httpapi

import (
	"fmt"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func RequireServiceToken(expected string, next http.Handler) http.Handler {
	if expected == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Token") != expected {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": "invalid_service_token",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// WithContentCache makes read-only content responses revalidatable.
//
// Every `/api/v1` read is derived from the in-memory snapshot, so the snapshot's
// content hash plus the request target identifies a response exactly: when the
// hash is unchanged the body cannot have changed either. That turns a repeat
// read into a 304 with no body, and lets the caller (the portal's incremental
// cache, and any CDN in between) hold the response until the next content
// release instead of re-fetching it per render.
//
// `ttl` should track DOCS_RELOAD_INTERVAL: a response cannot go stale faster
// than the snapshot behind it is rebuilt.
func WithContentCache(contentHash func() string, ttl time.Duration, next http.Handler) http.Handler {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	seconds := int(ttl.Seconds())
	cacheControl := fmt.Sprintf(
		"public, max-age=0, s-maxage=%d, stale-while-revalidate=%d",
		seconds, seconds*2,
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}

		hash := contentHash()
		if hash == "" {
			next.ServeHTTP(w, r)
			return
		}

		etag := contentETag(hash, r)
		header := w.Header()
		header.Set("ETag", etag)
		header.Set("Cache-Control", cacheControl)
		header.Set("X-Content-Hash", hash)
		// Language can also be resolved from headers when `?lang=` is absent, so
		// a shared cache must key on them too.
		header.Set("Vary", "Accept-Language, X-Language")

		if matchesETag(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func contentETag(hash string, r *http.Request) string {
	digest := fnv.New64a()
	for _, part := range []string{r.URL.Path, r.URL.RawQuery, resolveLang(r)} {
		_, _ = digest.Write([]byte(part))
		_, _ = digest.Write([]byte{0})
	}
	return `"` + hash + "-" + strconv.FormatUint(digest.Sum64(), 36) + `"`
}

func matchesETag(header, etag string) bool {
	if header == "" {
		return false
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}
