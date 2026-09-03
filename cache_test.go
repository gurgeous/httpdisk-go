package httpdisk

import (
	"bytes"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func newTestPayload(status int, body string) *Payload {
	return &Payload{
		Comment: "GET http://a.com/b",
		Status:  status,
		Reason:  http.StatusText(status),
		Header:  http.Header{"Content-Type": []string{"text/plain"}},
		Body:    []byte(body),
	}
}

func TestCacheGetSet(t *testing.T) {
	c := newCache(Options{Dir: t.TempDir()})

	ck := MustCacheKey(MustRequest("GET", "http://a.com/b"))

	// miss
	payload, status, err := c.Get(ck)
	assert.Nil(t, err)
	assert.Nil(t, payload)
	assert.Equal(t, StatusMiss, status)

	// set
	assert.Nil(t, c.Set(ck, newTestPayload(200, "hello")))

	// hit
	payload, status, err = c.Get(ck)
	assert.Nil(t, err)
	assert.Equal(t, StatusHit, status)
	if assert.NotNil(t, payload) {
		assert.Equal(t, 200, payload.Status)
		assert.Equal(t, "OK", payload.Reason)
		assert.Equal(t, "hello", string(payload.Body))
		assert.Equal(t, "text/plain", payload.Header.Get("Content-Type"))
		assert.Equal(t, "GET http://a.com/b", payload.Comment)
	}

	// peek skips the body
	status, err = c.Status(ck)
	assert.Nil(t, err)
	assert.Equal(t, StatusHit, status)

	// delete
	assert.Nil(t, c.Delete(ck))
	_, status, _ = c.Get(ck)
	assert.Equal(t, StatusMiss, status)

	// delete is a noop when there's nothing there
	assert.Nil(t, c.Delete(ck))

	// RemoveAll unlinks everything
	assert.Nil(t, c.Set(ck, newTestPayload(200, "hello")))
	assert.Nil(t, c.RemoveAll())
	_, err = os.Stat(c.Dir)
	assert.True(t, os.IsNotExist(err))
}

func TestCacheStatus(t *testing.T) {
	dir := t.TempDir()

	ck := MustCacheKey(MustRequest("GET", "http://a.com/b"))

	// hit
	c := newCache(Options{Dir: dir})
	assert.Nil(t, c.Set(ck, newTestPayload(200, "hello")))
	status, _ := c.Status(ck)
	assert.Equal(t, StatusHit, status)

	// force
	forced := newCache(Options{Dir: dir, Force: true})
	status, _ = forced.Status(ck)
	assert.Equal(t, StatusForce, status)

	// stale
	stale := newCache(Options{Dir: dir, Expires: 1 * time.Nanosecond})
	time.Sleep(2 * time.Millisecond)
	status, _ = stale.Status(ck)
	assert.Equal(t, StatusStale, status)

	// error (http)
	assert.Nil(t, c.Set(ck, newTestPayload(404, "")))
	status, _ = c.Status(ck)
	assert.Equal(t, StatusError, status)

	// error (network)
	assert.Nil(t, c.Set(ck, PayloadFromError(errNoSuchHost)))
	status, _ = c.Status(ck)
	assert.Equal(t, StatusError, status)

	// ...but ForceErrors ignores errors
	forceErrors := newCache(Options{Dir: dir, ForceErrors: true})
	status, _ = forceErrors.Status(ck)
	assert.Equal(t, StatusForce, status)
}

func TestCacheAge(t *testing.T) {
	c := newCache(Options{Dir: t.TempDir()})

	ck := MustCacheKey(MustRequest("GET", "http://a.com/b"))
	assert.Equal(t, time.Duration(0), c.Age(ck))

	assert.Nil(t, c.Set(ck, newTestPayload(200, "hello")))
	assert.Greater(t, c.Age(ck), time.Duration(0))
}

// We write plain or gzip, but always read both.
func TestCacheCompress(t *testing.T) {
	dir := t.TempDir()

	plain := newCache(Options{Dir: dir})
	compressed := newCache(Options{Dir: dir, Compress: true})

	ckPlain := MustCacheKey(MustRequest("GET", "http://a.com/plain"))
	ckGzip := MustCacheKey(MustRequest("GET", "http://a.com/gzip"))
	assert.Nil(t, plain.Set(ckPlain, newTestPayload(200, "hello")))
	assert.Nil(t, compressed.Set(ckGzip, newTestPayload(200, "hello")))

	// only one of them is gzipped on disk
	assertGzipped := func(ck *CacheKey, want bool) {
		data, err := os.ReadFile(plain.diskpath(ck))
		assert.Nil(t, err)
		assert.Equal(t, want, bytes.HasPrefix(data, gzipMagic))
	}
	assertGzipped(ckPlain, false)
	assertGzipped(ckGzip, true)

	// ...but both caches can read both files
	for _, c := range []*Cache{plain, compressed} {
		for _, ck := range []*CacheKey{ckPlain, ckGzip} {
			payload, status, err := c.Get(ck)
			assert.Nil(t, err)
			assert.Equal(t, StatusHit, status)
			if assert.NotNil(t, payload) {
				assert.Equal(t, "hello", string(payload.Body))
				assert.Equal(t, "text/plain", payload.Header.Get("Content-Type"))
			}

			// peek works too
			status, err = c.Status(ck)
			assert.Nil(t, err)
			assert.Equal(t, StatusHit, status)
		}
	}
}

func TestCacheCorrupt(t *testing.T) {
	c := newCache(Options{Dir: t.TempDir()})

	ck := MustCacheKey(MustRequest("GET", "http://a.com/b"))
	MustWrite(t, c.diskpath(ck), "this is not a payload")
	_, _, err := c.Get(ck)
	assert.NotNil(t, err)

	// truncated gzip
	MustWrite(t, c.diskpath(ck), string(gzipMagic)+"truncated")
	_, _, err = c.Get(ck)
	assert.NotNil(t, err)
}
