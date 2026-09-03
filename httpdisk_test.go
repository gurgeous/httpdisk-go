package httpdisk

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

var errNoSuchHost = errors.New("no such host")

func TestHTTPDisk(t *testing.T) {
	client := setupClient(t, Options{})

	drainBody := func(resp *http.Response) string {
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		return string(data)
	}

	//
	// 1. miss
	//

	url := "http://example.com/get"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Get %s failed %s", url, err)
	}
	defer resp.Body.Close()
	assert.Equal(t, "body 1", drainBody(resp))
	assert.Equal(t, "1", resp.Header.Get("X-Request-Id"))

	//
	// 2. hit
	//

	resp, err = client.Get(url)
	if err != nil {
		t.Fatalf("Get %s failed %s", url, err)
	}
	defer resp.Body.Close()
	assert.Equal(t, "body 1", drainBody(resp))
	assert.Equal(t, "1", resp.Header.Get("X-Request-Id"))
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "200 OK", resp.Status)
}

func TestHTTPDiskForce(t *testing.T) {
	client := setupClient(t, Options{Force: true})

	//
	// 1. miss
	//

	url := "http://example.com/get"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Get %s failed %s", url, err)
	}
	defer resp.Body.Close()
	assert.Equal(t, "1", resp.Header.Get("X-Request-Id"))

	//
	// 2. force second request
	//

	resp, err = client.Get(url)
	if err != nil {
		t.Fatalf("Get %s failed %s", url, err)
	}
	defer resp.Body.Close()
	assert.Equal(t, "2", resp.Header.Get("X-Request-Id"))
}

func TestHTTPDiskErrors(t *testing.T) {
	client := setupClient(t, Options{})

	var resp *http.Response
	var err error

	// Nework errors are tested elsewhere. See TestHTTPDiskTimeout and TestHTTPDiskNoSuchHost.

	// 40x error
	url := "http://httpbingo.org/status/404"
	resp, err = client.Get(url)
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "1", resp.Header.Get("X-Request-Id"))

	resp, err = client.Get(url)
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "1", resp.Header.Get("X-Request-Id"), "response not cached")

	// 50x error
	url = "http://httpbingo.org/status/502"
	resp, err = client.Get(url)
	assert.Nil(t, err)
	assert.Equal(t, 502, resp.StatusCode)
	assert.Equal(t, "2", resp.Header.Get("X-Request-Id"))

	resp, err = client.Get(url)
	assert.Nil(t, err)
	assert.Equal(t, 502, resp.StatusCode)
	assert.Equal(t, "2", resp.Header.Get("X-Request-Id"), "response not cached")
}

func TestHTTPDiskTimeout(t *testing.T) {
	// This test does not use vcr
	hd := NewHTTPDisk(Options{Dir: t.TempDir()})

	// Fake the error unless we're really hitting the network
	if os.Getenv("USE_NETWORK") == "" {
		hd.Transport = &errorRoundTripper{"context deadline exceeded"}
	}

	client := http.Client{Transport: hd, Timeout: 500 * time.Millisecond}

	url := "http://httpbingo.org/delay/1"
	_, err := client.Get(url)
	assert.NotNil(t, err)

	_, err = client.Get(url)
	if assert.NotNil(t, err) {
		assert.Contains(t, err.Error(), "(cached)", "%s error was not cached", url)
	}

	// cached network errors are stored as 999
	status, err := hd.Status(MustRequest("GET", url))
	assert.Nil(t, err)
	assert.Equal(t, StatusError, status.Status)
}

func TestHTTPDiskNoSuchHost(t *testing.T) {
	// This test does not use vcr
	hd := NewHTTPDisk(Options{Dir: t.TempDir()})

	// Fake the error unless we're really hitting the network
	if os.Getenv("USE_NETWORK") == "" {
		hd.Transport = &errorRoundTripper{"no such host"}
	}

	client := http.Client{Transport: hd}

	url := "http://bogus.bogus"
	_, err := client.Get(url)
	assert.NotNil(t, err)

	_, err = client.Get(url)
	if assert.NotNil(t, err) {
		assert.Contains(t, err.Error(), "(cached)", "%s error was not cached", url)
	}
}

// Errors we don't recognize are not cached
func TestHTTPDiskUnknownError(t *testing.T) {
	hd := NewHTTPDisk(Options{Dir: t.TempDir()})
	hd.Transport = &errorRoundTripper{"something we've never seen before"}

	client := http.Client{Transport: hd}

	url := "http://bogus.bogus"
	_, err := client.Get(url)
	assert.NotNil(t, err)

	status, err := hd.Status(MustRequest("GET", url))
	assert.Nil(t, err)
	assert.Equal(t, StatusMiss, status.Status)
}

func TestHTTPDiskForceErrors(t *testing.T) {
	client := setupClient(t, Options{ForceErrors: true})

	var resp *http.Response
	var err error

	// Nework errors are tested elsewhere. See TestHTTPDiskForceTimeout and TestHTTPDiskForceNoSuchHost.

	// 40x error
	url := "http://httpbingo.org/status/404"
	resp, err = client.Get(url)
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "1", resp.Header.Get("X-Request-Id"))

	resp, err = client.Get(url)
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "2", resp.Header.Get("X-Request-Id"), "response cached")

	// 50x error
	url = "http://httpbingo.org/status/502"
	resp, err = client.Get(url)
	assert.Nil(t, err)
	assert.Equal(t, 502, resp.StatusCode)
	assert.Equal(t, "3", resp.Header.Get("X-Request-Id"))

	resp, err = client.Get(url)
	assert.Nil(t, err)
	assert.Equal(t, 502, resp.StatusCode)
	assert.Equal(t, "4", resp.Header.Get("X-Request-Id"), "response cached")
}

func TestHTTPDiskForceTimeout(t *testing.T) {
	// This test does not use vcr
	hd := NewHTTPDisk(Options{Dir: t.TempDir(), ForceErrors: true})

	// Fake the error unless we're really hitting the network
	if os.Getenv("USE_NETWORK") == "" {
		hd.Transport = &errorRoundTripper{"context deadline exceeded"}
	}

	client := http.Client{Transport: hd, Timeout: 500 * time.Millisecond}

	url := "http://httpbingo.org/delay/1"
	client.Get(url)
	_, err := client.Get(url)
	if assert.NotNil(t, err) {
		assert.NotContains(t, err.Error(), "(cached)", "%s ForceErrors not honored", url)
	}
}

func TestHTTPDiskForceNoSuchHost(t *testing.T) {
	// This test does not use vcr
	hd := NewHTTPDisk(Options{Dir: t.TempDir(), ForceErrors: true})

	// Fake the error unless we're really hitting the network
	if os.Getenv("USE_NETWORK") == "" {
		hd.Transport = &errorRoundTripper{"no such host"}
	}

	client := http.Client{Transport: hd}

	url := "http://bogus.bogus"
	client.Get(url)
	_, err := client.Get(url)
	if assert.NotNil(t, err) {
		assert.NotContains(t, err.Error(), "(cached)", "%s ForceErrors not honored", url)
	}
}

func TestHTTPDiskExpires(t *testing.T) {
	client := setupClient(t, Options{Expires: 100 * time.Millisecond})

	//
	// 1. miss
	//

	url := "http://httpbingo.org/get"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Get %s failed %s", url, err)
	}
	defer resp.Body.Close()
	assert.Equal(t, "1", resp.Header.Get("X-Request-Id"))

	//
	// 2. hit
	//

	resp, err = client.Get(url)
	if err != nil {
		t.Fatalf("Get %s failed %s", url, err)
	}
	defer resp.Body.Close()
	assert.Equal(t, "1", resp.Header.Get("X-Request-Id"))

	//
	// 3. stale, re-fetch
	//

	time.Sleep(150 * time.Millisecond)

	resp, err = client.Get(url)
	if err != nil {
		t.Fatalf("Get %s failed %s", url, err)
	}
	defer resp.Body.Close()
	assert.Equal(t, "2", resp.Header.Get("X-Request-Id"))
}

func TestHTTPDiskStatus(t *testing.T) {
	hd := NewHTTPDisk(Options{Dir: t.TempDir()})

	req := MustRequest("GET", "http://httpbingo.org/get")

	// 1. miss
	status, err := hd.Status(req)
	assert.Nil(t, err)
	assert.Equal(t, StatusMiss, status.Status)
	assert.Equal(t, "GET http://httpbingo.org/get", status.Key)
	assert.Equal(t, sha256String(status.Key), status.Digest)

	// 2. hit
	MustWrite(t, status.Path, "# GET http://httpbingo.org/get\nHTTPDISK 200 OK\n\nhello")
	status, err = hd.Status(req)
	assert.Nil(t, err)
	assert.Equal(t, StatusHit, status.Status)

	// 3. http error
	MustWrite(t, status.Path, "# GET http://httpbingo.org/get\nHTTPDISK 404 Not Found\n\n")
	status, err = hd.Status(req)
	assert.Nil(t, err)
	assert.Equal(t, StatusError, status.Status)

	// 4. network error
	MustWrite(t, status.Path, "# GET http://httpbingo.org/get\nHTTPDISK 999 no such host\n\n")
	status, err = hd.Status(req)
	assert.Nil(t, err)
	assert.Equal(t, StatusError, status.Status)

	// 5. stale
	hd.Cache.Expires = 1 * time.Nanosecond
	time.Sleep(2 * time.Millisecond)
	status, err = hd.Status(req)
	assert.Nil(t, err)
	assert.Equal(t, StatusStale, status.Status)
}

// HTTPDISK_DEBUG turns on logging to stderr
func TestHTTPDiskDebugEnv(t *testing.T) {
	hd := NewHTTPDisk(Options{Dir: t.TempDir()})
	assert.Nil(t, hd.Options.Logger)

	t.Setenv("HTTPDISK_DEBUG", "1")
	hd = NewHTTPDisk(Options{Dir: t.TempDir()})
	assert.NotNil(t, hd.Options.Logger)

	// an explicit logger wins
	logger := log.New(io.Discard, "", 0)
	hd = NewHTTPDisk(Options{Dir: t.TempDir(), Logger: logger})
	assert.Equal(t, logger, hd.Options.Logger)
}

// We log hit/miss for every request
func TestHTTPDiskLogging(t *testing.T) {
	buf := &bytes.Buffer{}
	hd := NewHTTPDisk(Options{Dir: t.TempDir(), Logger: log.New(buf, "", 0)})
	hd.Transport = &errorRoundTripper{"no such host"}

	client := http.Client{Transport: hd}
	client.Get("http://bogus.bogus/x")
	client.Get("http://bogus.bogus/x")

	assert.Contains(t, buf.String(), "GET http://bogus.bogus/x (miss)")
	assert.Contains(t, buf.String(), "GET http://bogus.bogus/x (error)")
}

//
// Helpers
//

// Create and configure an http client for testing using httpdisk and recorder
// for the current test.
//
// Fixtures must be stored in testdata/<testName>.yaml
//
// The cassette and the temp cache dir are cleaned up automatically.
func setupClient(t *testing.T, hdOptions Options) *http.Client {
	// Default options
	if hdOptions.Dir == "" {
		hdOptions.Dir = t.TempDir()
	}

	hd := NewHTTPDisk(hdOptions)
	hd.Transport = newVCR(t)

	return &http.Client{Transport: hd}
}

// Create a new recorder for the current test.
// Fixtures must be stored in testdata/<testName>.yaml
//
// The recorder is stopped automatically when the test finishes.
func newVCR(t *testing.T) *recorder.Recorder {
	name := fmt.Sprintf("testdata/%s", t.Name())

	// ModeReplayOnly so a missing cassette fails instead of quietly recording
	// from the network. Our cassettes are hand written, so we match on method
	// and url only instead of using the strict default matcher.
	vcr, err := recorder.New(name,
		recorder.WithMode(recorder.ModeReplayOnly),
		recorder.WithMatcher(func(r *http.Request, i cassette.Request) bool {
			return r.Method == i.Method && r.URL.String() == i.URL
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := vcr.Stop(); err != nil {
			t.Error(err)
		}
	})

	return vcr
}

//
// Custom RoundTripper that always returns an error
//

type errorRoundTripper struct{ errorString string }

func (t *errorRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, errors.New(t.errorString)
}
