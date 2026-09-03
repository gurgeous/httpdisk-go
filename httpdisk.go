package httpdisk

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HTTPDisk is a caching http transport.
type HTTPDisk struct {
	// Underlying Cache.
	Cache Cache
	// if nil, http.DefaultTransport is used.
	Transport http.RoundTripper
	Options   Options
}

// Options for creating a new HTTPDisk.
type Options struct {
	// Directory where the cache is stored. Defaults to ~/httpdisk-go.
	Dir string

	// If true, gzip cache files as we write them. Defaults to false. Note that
	// we always read both compressed and plain files, so this can be flipped
	// without invalidating an existing cache.
	Compress bool

	// Maximum amount of time a cached response is considered fresh. If less
	// than or equal to zero, then all content is considered fresh. If positive,
	// then cached content will be re-fetched if it is older than this.
	Expires time.Duration

	// Don't read anything from cache (but still write)
	Force bool

	// Don't read errors from cache (but still write)
	ForceErrors bool

	// Query and form params to ignore when calculating cache keys.
	IgnoreParams []string

	// Optional logger. If nil and HTTPDISK_DEBUG is set, we log to stderr.
	Logger *log.Logger
}

// Status of a request in the cache.
type Status struct {
	Age    time.Duration
	Digest string
	Key    string
	Path   string
	Status string
	URL    string
}

// NewHTTPDisk constructs a new HTTPDisk.
func NewHTTPDisk(options Options) *HTTPDisk {
	if options.Dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		options.Dir = filepath.Join(home, "httpdisk-go")
	}
	if options.Logger == nil && os.Getenv("HTTPDISK_DEBUG") != "" {
		options.Logger = log.New(os.Stderr, "httpdisk: ", 0)
	}
	return &HTTPDisk{Cache: *newCache(options), Options: options}
}

// Status returns the cache status for a request.
func (hd *HTTPDisk) Status(req *http.Request) (*Status, error) {
	cacheKey, err := hd.cacheKey(req)
	if err != nil {
		return nil, err
	}

	status, err := hd.Cache.Status(cacheKey)
	if err != nil {
		return nil, err
	}

	return &Status{
		Age:    hd.Cache.Age(cacheKey),
		Digest: cacheKey.Digest(),
		Key:    cacheKey.Key(),
		Path:   hd.Cache.diskpath(cacheKey),
		Status: status,
		URL:    req.URL.String(),
	}, nil
}

func (hd *HTTPDisk) RoundTrip(req *http.Request) (*http.Response, error) {
	cacheKey, err := hd.cacheKey(req)
	if err != nil {
		return nil, err
	}

	// check the cache
	payload, status, err := hd.Cache.Get(cacheKey)
	hd.logf("%s %s (%s)", req.Method, req.URL, status)
	if err != nil {
		return nil, err
	}

	if payload != nil {
		// cached network errors are returned as errors, not responses
		if payload.IsNetworkError() {
			return nil, fmt.Errorf("%s (cached)", payload.Reason)
		}
		return payload.Response(req), nil
	}

	return hd.fetch(req, cacheKey)
}

//
// helpers
//

// Fetch a response over the network and cache it.
func (hd *HTTPDisk) fetch(req *http.Request, cacheKey *CacheKey) (*http.Response, error) {
	transport := hd.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	start := time.Now()
	resp, err := transport.RoundTrip(req)
	if err != nil {
		hd.logf("network error on %s (%s)", req.URL, err)
		return nil, hd.setError(cacheKey, req, err)
	}
	if resp.StatusCode >= 400 {
		hd.logf("http error on %s (%s)", req.URL, resp.Status)
	}

	// our headers
	elapsed := float64(time.Since(start)) / float64(time.Second)
	resp.Header.Set("X-Httpdisk-Elapsed", fmt.Sprintf("%0.3f", elapsed))
	resp.Header.Set("X-Httpdisk-Url", req.URL.String())

	// drain the body and cache. errors can occur here if the server returns an
	// invalid body, so consider caching those too
	payload, err := PayloadFromResponse(resp)
	if err != nil {
		hd.logf("body error on %s (%s)", req.URL, err)
		return nil, hd.setError(cacheKey, req, err)
	}
	payload.Comment = fmt.Sprintf("%s %s", strings.ToUpper(req.Method), req.URL)
	if err := hd.Cache.Set(cacheKey, payload); err != nil {
		return nil, err
	}

	return resp, nil
}

// Cache err as a 999 response if we can, then return it. Errors while writing
// to the cache take precedence, so the caller has a chance to see them.
func (hd *HTTPDisk) setError(cacheKey *CacheKey, req *http.Request, err error) error {
	if !hd.isCacheableError(err) {
		return err
	}
	payload := PayloadFromError(err)
	payload.Comment = fmt.Sprintf("%s %s", strings.ToUpper(req.Method), req.URL)
	if err2 := hd.Cache.Set(cacheKey, payload); err2 != nil {
		return err2
	}
	return err
}

func (hd *HTTPDisk) cacheKey(req *http.Request) (*CacheKey, error) {
	return NewCacheKey(req, hd.Options.IgnoreParams)
}

func (hd *HTTPDisk) logf(format string, args ...any) {
	if hd.Options.Logger != nil {
		hd.Options.Logger.Printf(format, args...)
	}
}

// if err.Error() contains one of these, we consider the error to be cacheable
// and we write it to disk.
var cacheableErrors = []string{
	"certificate has expired",
	"certificate is valid",
	"certificate signed by unknown authority",
	"connection refused",
	"connection reset by peer",
	"context deadline exceeded",
	"EOF",
	"handshake failure",
	"i/o timeout",
	"no route to host",
	"no such host",
	"request canceled",
	"stream error",
	"tls: internal error",
	"tls: unrecognized name",
}

func (hd *HTTPDisk) isCacheableError(err error) bool {
	errorString := err.Error()
	for _, s := range cacheableErrors {
		if strings.Contains(errorString, s) {
			return true
		}
	}

	hd.logf("not caching unknown error type:%T v:%v", err, err)
	return false
}
