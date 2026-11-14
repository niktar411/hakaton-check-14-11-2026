package operations

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// maxBufferedBody bounds the body we are willing to buffer for ETag
// computation. Larger responses stream through untouched.
const maxBufferedBody = 1 << 20 // 1 MiB

// cacheableControl is the cache policy for public GET responses: a short
// client TTL, a stale window for the CDN tier.
const cacheableControl = "public, max-age=60, stale-while-revalidate=300"

// cachePublic makes cacheable public GET responses revalidatable. Public
// SAM/planner/question payloads change rarely, while every visit refetches
// them — this turns most of those round trips into either a browser cache
// hit or a 304 with no body. Successful responses get the cache policy and
// a strong ETag; failures stay uncacheable.
func cachePublic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Range semantics are not implemented; such requests pass through.
		if r.Method != http.MethodGet || r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}
		rec := &cacheRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if !rec.bufferedOK() {
			rec.flush()
			return
		}
		w.Header().Set("Cache-Control", cacheableControl)
		etag := rec.etag()
		w.Header().Set("ETag", etag)
		if ifNoneMatch(r) == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		rec.flush()
	})
}

// ifNoneMatch returns the client's If-None-Match value with W/ prefixes and
// quotes normalized away, or "" when absent. Exact single-entity comparison
// is enough here: we always serve one strong ETag per resource.
func ifNoneMatch(r *http.Request) string {
	v := r.Header.Get("If-None-Match")
	if v == "" {
		return ""
	}
	v = strings.TrimPrefix(v, "W/")
	return `"` + strings.Trim(v, `"`) + `"`
}

// cacheRecorder buffers the handler output (up to maxBufferedBody) so the
// ETag can be computed and a 304 can be answered without a body. When the
// limit is exceeded it switches to streaming passthrough and skips caching.
type cacheRecorder struct {
	http.ResponseWriter
	status   int
	buf      []byte
	tooLarge bool
	flushed  bool
}

func (c *cacheRecorder) WriteHeader(status int) {
	c.status = status
}

func (c *cacheRecorder) Write(p []byte) (int, error) {
	if c.tooLarge {
		c.flush()
		return c.ResponseWriter.Write(p)
	}
	if len(c.buf)+len(p) > maxBufferedBody {
		c.tooLarge = true
		c.flush()
		return c.ResponseWriter.Write(p)
	}
	c.buf = append(c.buf, p...)
	return len(p), nil
}

// bufferedOK reports whether the recorded response can be decorated with
// cache metadata: a buffered successful response that is still intact.
func (c *cacheRecorder) bufferedOK() bool {
	return c.status == http.StatusOK && !c.tooLarge && !c.flushed
}

// etag returns the strong ETag of the buffered body.
func (c *cacheRecorder) etag() string {
	sum := sha256.Sum256(c.buf)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// flush passes the recorded response to the real writer exactly once. It
// must not be called before cache headers are final.
func (c *cacheRecorder) flush() {
	if c.flushed {
		return
	}
	c.flushed = true
	status := c.status
	if status == 0 {
		status = http.StatusOK
	}
	c.ResponseWriter.WriteHeader(status)
	_, _ = c.ResponseWriter.Write(c.buf)
	c.buf = nil
}
