package httpdisk

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// cache statuses
const (
	StatusError = "error"
	StatusForce = "force"
	StatusHit   = "hit"
	StatusMiss  = "miss"
	StatusStale = "stale"
)

// Cache will cache http responses on disk, using the http.Request to calculate
// a key. It deals with keys and files, not the network.
type Cache struct {
	// Directory where the cache is stored.
	Dir string

	// If true, gzip cache files as we write them. We can always read both
	// compressed and plain files, regardless of this setting.
	Compress bool

	// Cached responses older than this are considered stale. Zero means never
	// expire.
	Expires time.Duration

	// Don't read anything from cache (but still write)
	Force bool

	// Don't read errors from cache (but still write)
	ForceErrors bool
}

func newCache(options Options) *Cache {
	return &Cache{
		Dir:         options.Dir,
		Compress:    options.Compress,
		Expires:     options.Expires,
		Force:       options.Force,
		ForceErrors: options.ForceErrors,
	}
}

// Get the cached payload for a request, along with the cache status. The
// payload is nil unless the status is hit or error.
func (cache *Cache) Get(cacheKey *CacheKey) (*Payload, string, error) {
	return cache.get(cacheKey, false)
}

// Status returns the cache status for a request, one of error, force, hit,
// miss or stale.
func (cache *Cache) Status(cacheKey *CacheKey) (string, error) {
	_, status, err := cache.get(cacheKey, true)
	return status, err
}

// Set the cached payload for a request.
func (cache *Cache) Set(cacheKey *CacheKey, payload *Payload) error {
	// make sure directory exists
	diskpath := cache.diskpath(cacheKey)
	if err := os.MkdirAll(filepath.Dir(diskpath), 0o755); err != nil {
		return err
	}

	// write to tmp file in same directory
	tmp := filepath.Join(filepath.Dir(diskpath), fmt.Sprintf(".tmp-%s", filepath.Base(diskpath)))
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)

	if err := cache.writePayload(f, payload); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// move into place
	return os.Rename(tmp, diskpath)
}

// Delete the cached response for a request, if any.
func (cache *Cache) Delete(cacheKey *CacheKey) error {
	err := os.Remove(cache.diskpath(cacheKey))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RemoveAll unlinks the cache.
func (cache *Cache) RemoveAll() error {
	return os.RemoveAll(cache.Dir)
}

// Age of the cached response for a request. Zero if not cached.
func (cache *Cache) Age(cacheKey *CacheKey) time.Duration {
	stat, err := os.Stat(cache.diskpath(cacheKey))
	if err != nil {
		return 0
	}
	return time.Since(stat.ModTime())
}

//
// helpers
//

// low level read, returns the payload and the cache status
func (cache *Cache) get(cacheKey *CacheKey, peek bool) (*Payload, string, error) {
	path := cache.diskpath(cacheKey)

	stat, err := os.Stat(path)
	if err != nil {
		return nil, StatusMiss, nil
	}
	if cache.expired(stat) {
		return nil, StatusStale, nil
	}
	if cache.Force {
		return nil, StatusForce, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, StatusMiss, nil
	}
	defer f.Close()

	payload, err := readPayload(f, peek)
	if err != nil {
		return nil, StatusMiss, fmt.Errorf("%s: %s", path, err)
	}

	if payload.IsError() {
		if cache.ForceErrors {
			return nil, StatusForce, nil
		}
		return payload, StatusError, nil
	}

	return payload, StatusHit, nil
}

func (cache *Cache) writePayload(w io.Writer, payload *Payload) error {
	if !cache.Compress {
		return payload.Write(w)
	}
	gz := gzip.NewWriter(w)
	if err := payload.Write(gz); err != nil {
		return err
	}
	return gz.Close()
}

// gzip magic, see https://www.rfc-editor.org/rfc/rfc1952#page-6
var gzipMagic = []byte{0x1f, 0x8b}

// Read a payload, transparently handling both compressed and plain files.
func readPayload(r io.Reader, peek bool) (*Payload, error) {
	buf := bufio.NewReader(r)

	magic, err := buf.Peek(2)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if bytes.Equal(magic, gzipMagic) {
		gz, err := gzip.NewReader(buf)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		return ReadPayload(gz, peek)
	}

	return ReadPayload(buf, peek)
}

func (cache *Cache) expired(stat os.FileInfo) bool {
	return cache.Expires > 0 && time.Since(stat.ModTime()) > cache.Expires
}

func (cache *Cache) diskpath(cacheKey *CacheKey) string {
	return filepath.Join(cache.Dir, cacheKey.Diskpath())
}
