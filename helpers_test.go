package httpdisk

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// create cache key
func MustCacheKey(req *http.Request, ignoreParams ...string) *CacheKey {
	ck, err := NewCacheKey(req, ignoreParams)
	if err != nil {
		panic(err)
	}
	return ck
}

// create a test request
func MustRequest(method string, url string) *http.Request {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		panic(err)
	}
	return req
}

// write data to path
func MustWrite(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
