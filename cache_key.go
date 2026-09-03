package httpdisk

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DefaultIgnoreParams are query/form params that are dropped when calculating
// cache keys. These typically vary from request to request, which would
// otherwise fragment the cache. Set Options.IgnoreParams to override, or to an
// empty (non-nil) slice to disable.
var DefaultIgnoreParams = []string{
	"_",
	"access_token",
	"api_key",
	"apikey",
	"auth_token",
	"key",
	"nonce",
	"sig",
	"signature",
	"timestamp",
	"token",
}

// bodies at least this long are hashed instead of being included verbatim
const maxBodyLen = 50

const formContentType = "application/x-www-form-urlencoded"

// a key in the cache
type CacheKey struct {
	Request *http.Request

	// query params to ignore when calculating the key
	IgnoreParams []string
}

func NewCacheKey(req *http.Request, ignoreParams []string) (*CacheKey, error) {
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return nil, fmt.Errorf("http/https required (%s)", req.URL.String())
	}
	if req.URL.Host == "" {
		return nil, fmt.Errorf("host required (%s)", req.URL.String())
	}
	return &CacheKey{Request: req, IgnoreParams: ignoreParams}, nil
}

// Key calculates a canonical cache key for the request based on the http
// method, the normalized URL, and the request body if present. The key can be
// quite long since it contains the request body.
func (cacheKey *CacheKey) Key() string {
	req := cacheKey.Request

	method := strings.ToUpper(req.Method)
	if method == "" {
		method = "GET"
	}
	scheme := strings.ToLower(req.URL.Scheme)

	key := []string{method, " ", scheme, "://", strings.ToLower(req.URL.Hostname())}
	if port := req.URL.Port(); port != "" && port != defaultPorts[scheme] {
		key = append(key, ":", port)
	}
	if path := req.URL.EscapedPath(); path != "" && path != "/" {
		key = append(key, path)
	}
	if query := cacheKey.canonicalQuery(req.URL.RawQuery); query != "" {
		key = append(key, "?", query)
	}
	if body, ok := cacheKey.bodykey(); ok {
		key = append(key, " ", body)
	}

	return strings.Join(key, "")
}

// Digest returns the md5 sum for this request.
func (cacheKey *CacheKey) Digest() string {
	return md5String(cacheKey.Key())
}

// Diskpath returns the relative path on disk for this request.
func (cacheKey *CacheKey) Diskpath() string {
	digest := cacheKey.Digest()
	return filepath.Join(hostdir(cacheKey.Request.URL.Hostname()), digest[0:3], digest[3:])
}

//
// helpers
//

var defaultPorts = map[string]string{
	"http":  "80",
	"https": "443",
}

// Calculate the cache key segment for the request body. Form bodies are
// canonicalized, long bodies are hashed. The bool is false if this request
// has no body at all.
func (cacheKey *CacheKey) bodykey() (string, bool) {
	req := cacheKey.Request
	if req.GetBody == nil {
		return "", false
	}
	reader, err := req.GetBody()
	if err != nil {
		return "", false
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", false
	}
	body := string(data)

	if req.Header.Get("Content-Type") == formContentType {
		// canonicalized away to nothing? then there's no body segment
		body = cacheKey.canonicalQuery(body)
		return body, body != ""
	}
	if len(body) >= maxBodyLen {
		return md5String(body), true
	}
	return body, true
}

// Sort query params and drop the ones we've been asked to ignore. Note that we
// sort the raw "key=value" strings without decoding them, to match the ruby
// httpdisk gem.
func (cacheKey *CacheKey) canonicalQuery(query string) string {
	if query == "" {
		return ""
	}

	parts := strings.Split(query, "&")
	sort.Strings(parts)

	if len(cacheKey.IgnoreParams) > 0 {
		ignore := map[string]bool{}
		for _, param := range cacheKey.IgnoreParams {
			ignore[url.QueryEscape(param)] = true
		}
		kept := parts[:0]
		for _, part := range parts {
			name, _, _ := strings.Cut(part, "=")
			if !ignore[name] {
				kept = append(kept, part)
			}
		}
		parts = kept
	}

	return strings.Join(parts, "&")
}

var (
	hostWwwRe   = regexp.MustCompile(`^www\.`)
	hostCharsRe = regexp.MustCompile(`[^a-z0-9._-]+`)
	hostDotsRe  = regexp.MustCompile(`\.{2,}`)
)

// Calculate a nice directory name from the hostname. Collisions are ok because
// the rest of the path is an md5 checksum.
func hostdir(host string) string {
	host = strings.ToLower(host)
	host = hostWwwRe.ReplaceAllString(host, "")
	host = hostCharsRe.ReplaceAllString(host, "")
	host = hostDotsRe.ReplaceAllString(host, ".")
	if host == "" {
		host = "any"
	}
	return host
}

func md5String(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}
