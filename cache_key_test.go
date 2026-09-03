package httpdisk

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Diskpath calculation.
func TestCacheKeyDiskpath(t *testing.T) {
	ck := MustCacheKey(MustRequest("GET", "http://google.com/?q=test"))
	digest := ck.Digest()
	assert.Equal(t, "3e0e77d99dbe211fe16fa8e4916a2ac14484f492b0ecce7f360f5294af52885d", digest)
	expected := filepath.Join("google.com", digest[:3], digest[3:])
	assert.Equal(t, expected, ck.Diskpath())
}

// key normalization
func TestCacheKeys(t *testing.T) {
	assertMatch := func(a *http.Request, b *http.Request) {
		c1, c2 := MustCacheKey(a), MustCacheKey(b)
		assert.Equal(t, c1.Key(), c2.Key())
	}
	assertDiffer := func(a *http.Request, b *http.Request) {
		c1, c2 := MustCacheKey(a), MustCacheKey(b)
		assert.NotEqual(t, c1.Key(), c2.Key())
	}

	// these pairs should match
	match := [][]string{
		{"http://a.com?a=1&a=2&b=2&c=3", "HTTP://A.COM:80?c=3&b=2&a=2&a=1"},
		{"https://a.com?a=1&b=2&c=3", "HTTPs://A.COM:443?c=3&b=2&a=1"},
		{"https://a.com?", "HTTPs://A.COM:443/"},
	}
	for _, pair := range match {
		assertMatch(MustRequest("GET", pair[0]), MustRequest("GET", pair[1]))
	}

	// methods differ, keys differ
	assertDiffer(MustRequest("GET", "http://a.com"), MustRequest("HEAD", "http://a.com"))

	// bodies differ, keys differ
	req1, _ := http.NewRequest("POST", "http://a.com", strings.NewReader("abc"))
	req2, _ := http.NewRequest("POST", "http://a.com", strings.NewReader("def"))
	assertDiffer(req1, req2)
}

func TestCacheKeyIgnoreParams(t *testing.T) {
	key := func(url string, ignore ...string) string {
		return MustCacheKey(MustRequest("GET", url), ignore...).Key()
	}

	// ignored params don't affect the key
	assert.Equal(t,
		key("http://a.com/x?q=1&token=aaa", "token"),
		key("http://a.com/x?q=1&token=bbb", "token"))

	// ...but non-ignored params do
	assert.NotEqual(t,
		key("http://a.com/x?q=1&token=aaa", "nope"),
		key("http://a.com/x?q=1&token=bbb", "nope"))

	// no ignore_params, no filtering
	assert.Equal(t, "GET http://a.com/x?q=1&token=aaa", key("http://a.com/x?q=1&token=aaa"))

	// escaped param names are matched
	assert.Equal(t, "GET http://a.com/x", key("http://a.com/x?a+b=1", "a b"))

	// no ignore params by default
	hd := NewHTTPDisk(Options{Dir: t.TempDir()})
	ck, err := hd.cacheKey(MustRequest("GET", "http://a.com/x?api_key=secret&q=1"))
	assert.Nil(t, err)
	assert.Equal(t, "GET http://a.com/x?api_key=secret&q=1", ck.Key())

	// suggested ignore params are opt-in
	hd = NewHTTPDisk(Options{Dir: t.TempDir(), IgnoreParams: DefaultIgnoreParams})
	ck, err = hd.cacheKey(MustRequest("GET", "http://a.com/x?api_key=secret&q=1"))
	assert.Nil(t, err)
	assert.Equal(t, "GET http://a.com/x?q=1", ck.Key())
}

func TestCacheKeyErrors(t *testing.T) {
	_, err := NewCacheKey(MustRequest("GET", "ftp://a.com"), nil)
	assert.NotNil(t, err)

	_, err = NewCacheKey(MustRequest("GET", "/relative"), nil)
	assert.NotNil(t, err)
}

func TestCacheHost(t *testing.T) {
	sep := regexp.QuoteMeta(fmt.Sprintf("%c", os.PathSeparator))
	hostPathRE := regexp.MustCompile(fmt.Sprintf("^a\\.com%s[a-f0-9]{3}%s[a-f0-9]+$", sep, sep))

	urls := []string{"http://a.com", "http://www.a~~.com"}
	for _, url := range urls {
		ck := MustCacheKey(MustRequest("GET", url))
		assert.Regexp(t, hostPathRE, ck.Diskpath())
	}
}

func TestHostdir(t *testing.T) {
	assert.Equal(t, "a.com", hostdir("A.COM"))
	assert.Equal(t, "a.com", hostdir("www.a.com"))
	assert.Equal(t, "a.com", hostdir("www.a~~.com"))
	assert.Equal(t, "a.com", hostdir("a...com"))
	assert.Equal(t, "any", hostdir("~~~"))
}
