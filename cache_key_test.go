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

// These come from the ruby httpdisk gem, and must not change. See README.
func TestCacheKeyRubyParity(t *testing.T) {
	tests := []struct {
		method  string
		url     string
		body    string
		hasBody bool
		form    bool
		ignore  []string
		key     string
		digest  string
	}{
		{
			method: "GET", url: "http://google.com/?q=ruby",
			key: "GET http://google.com?q=ruby", digest: "0e37f96800a55958fa6029283c78f672",
		},
		{
			method: "GET", url: "http://a.com",
			key: "GET http://a.com", digest: "e1ed291c6e286d34a4476f11c026d507",
		},
		{
			method: "GET", url: "http://a.com/",
			key: "GET http://a.com", digest: "e1ed291c6e286d34a4476f11c026d507",
		},
		{
			method: "GET", url: "https://a.com/",
			key: "GET https://a.com", digest: "be7d2235dc0faefadb642fa683edd654",
		},
		{
			method: "GET", url: "http://A.COM:80/",
			key: "GET http://a.com", digest: "e1ed291c6e286d34a4476f11c026d507",
		},
		{
			method: "GET", url: "http://a.com:8080/",
			key: "GET http://a.com:8080", digest: "65050b3b12e888ef46dec44ab31335ba",
		},
		{
			method: "GET", url: "https://a.com:443/x",
			key: "GET https://a.com/x", digest: "fde7b9c0bcfb21814dd2262ab4a3d01f",
		},
		{
			method: "GET", url: "http://a.com?a=1&a=2&b=2&c=3",
			key: "GET http://a.com?a=1&a=2&b=2&c=3", digest: "52f99f69786c8b4c7689daeaac9f67a0",
		},
		{
			method: "GET", url: "http://a.com?c=3&b=2&a=2&a=1",
			key: "GET http://a.com?a=1&a=2&b=2&c=3", digest: "52f99f69786c8b4c7689daeaac9f67a0",
		},
		{
			method: "GET", url: "http://a.com?",
			key: "GET http://a.com", digest: "e1ed291c6e286d34a4476f11c026d507",
		},
		{
			method: "GET", url: "http://a.com/path%20x?y=a%20b",
			key: "GET http://a.com/path%20x?y=a%20b", digest: "eff8f67de764702abd91edaac8957d5d",
		},
		{
			method: "GET", url: "http://www.a~~.com/",
			key: "GET http://www.a~~.com", digest: "40c5a24b6fc6ba85c4f030e8989b8f6f",
		},
		{
			method: "GET", url: "http://a.com/x?api_key=secret&q=1", ignore: []string{"api_key"},
			key: "GET http://a.com/x?q=1", digest: "95912b49b2691c503090d460b4344c0c",
		},
		{
			method: "GET", url: "http://a.com/x?api_key=secret", ignore: []string{"api_key"},
			key: "GET http://a.com/x", digest: "369607115d4a1efec05ed1ce81e711cb",
		},
		{
			method: "HEAD", url: "http://a.com",
			key: "HEAD http://a.com", digest: "272597ee879e4a48e977af365a092895",
		},
		{
			method: "POST", url: "http://a.com", body: "abc", hasBody: true,
			key: "POST http://a.com abc", digest: "b423e6a05f91991e9dd6a69fea9010cf",
		},
		{
			method: "POST", url: "http://a.com", body: "", hasBody: true,
			key: "POST http://a.com ", digest: "a42a5924abe54eeee86dd1b6c16c996c",
		},
		{
			method: "POST", url: "http://a.com", body: "b=2&a=1", hasBody: true, form: true,
			key: "POST http://a.com a=1&b=2", digest: "65fc0e2036b0b62c98b3cf5451ee74be",
		},
		{
			method: "POST", url: "http://a.com", body: "a=1&api_key=x", hasBody: true, form: true, ignore: []string{"api_key"},
			key: "POST http://a.com a=1", digest: "7074f387176a2658d948e1c050530ee2",
		},
		{
			method: "POST", url: "http://a.com", body: strings.Repeat("x", 60), hasBody: true,
			key: "POST http://a.com 1198000c11968f9368e02d6da57ec147", digest: "46280a92dc14a2158a627da4ce4db6cd",
		},
		{
			method: "POST", url: "http://a.com", body: strings.Repeat("x", 49), hasBody: true,
			key: fmt.Sprintf("POST http://a.com %s", strings.Repeat("x", 49)), digest: "0ef1ae07891d71a3aee90205f255e853",
		},
	}

	for _, test := range tests {
		var req *http.Request
		var err error
		if test.hasBody {
			req, err = http.NewRequest(test.method, test.url, strings.NewReader(test.body))
		} else {
			req, err = http.NewRequest(test.method, test.url, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		if test.form {
			req.Header.Set("Content-Type", formContentType)
		}

		ck := MustCacheKey(req, test.ignore...)
		assert.Equal(t, test.key, ck.Key(), "key for %s %s", test.method, test.url)
		assert.Equal(t, test.digest, ck.Digest(), "digest for %s %s", test.method, test.url)
	}
}

// Diskpath must match the ruby httpdisk gem too.
func TestCacheKeyDiskpath(t *testing.T) {
	ck := MustCacheKey(MustRequest("GET", "http://google.com/?q=ruby"))
	expected := filepath.Join("google.com", "0e3", "7f96800a55958fa6029283c78f672")
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

	// DefaultIgnoreParams is used by NewHTTPDisk, not by NewCacheKey
	hd := NewHTTPDisk(Options{Dir: t.TempDir()})
	ck, err := hd.cacheKey(MustRequest("GET", "http://a.com/x?api_key=secret&q=1"))
	assert.Nil(t, err)
	assert.Equal(t, "GET http://a.com/x?q=1", ck.Key())

	// ...and can be disabled with an empty slice
	hd = NewHTTPDisk(Options{Dir: t.TempDir(), IgnoreParams: []string{}})
	ck, err = hd.cacheKey(MustRequest("GET", "http://a.com/x?api_key=secret&q=1"))
	assert.Nil(t, err)
	assert.Equal(t, "GET http://a.com/x?api_key=secret&q=1", ck.Key())
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
