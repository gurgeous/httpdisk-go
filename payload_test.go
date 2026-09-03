package httpdisk

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPayloadRoundTrip(t *testing.T) {
	payload := &Payload{
		Comment: "GET http://a.com/b",
		Status:  200,
		Reason:  "OK",
		Header: http.Header{
			"Content-Type": []string{"text/html"},
			"Set-Cookie":   []string{"a=1", "b=2"},
		},
		Body: []byte("hello\nthere"),
	}

	buf := &bytes.Buffer{}
	assert.Nil(t, payload.Write(buf))

	expected := strings.Join([]string{
		"# GET http://a.com/b",
		"HTTPDISK 200 OK",
		"Content-Type: text/html",
		"Set-Cookie: a=1",
		"Set-Cookie: b=2",
		"",
		"hello\nthere",
	}, "\n")
	assert.Equal(t, expected, buf.String())

	got, err := ReadPayload(bytes.NewReader(buf.Bytes()), false)
	assert.Nil(t, err)
	assert.Equal(t, payload.Comment, got.Comment)
	assert.Equal(t, payload.Status, got.Status)
	assert.Equal(t, payload.Reason, got.Reason)
	assert.Equal(t, payload.Header, got.Header)
	assert.Equal(t, payload.Body, got.Body)

	// peek skips the body
	got, err = ReadPayload(bytes.NewReader(buf.Bytes()), true)
	assert.Nil(t, err)
	assert.Equal(t, 200, got.Status)
	assert.Nil(t, got.Body)
}

// This is the format used by the ruby httpdisk gem.
func TestPayloadReadRuby(t *testing.T) {
	data := "# GET http://www.google.com\nHTTPDISK 200 OK\ndate: Mon, 19 Apr 2021 18:40:01 GMT\nexpires: -1\n\n<html>"

	payload, err := ReadPayload(strings.NewReader(data), false)
	assert.Nil(t, err)
	assert.Equal(t, "GET http://www.google.com", payload.Comment)
	assert.Equal(t, 200, payload.Status)
	assert.Equal(t, "OK", payload.Reason)
	assert.Equal(t, "Mon, 19 Apr 2021 18:40:01 GMT", payload.Header.Get("Date"))
	assert.Equal(t, "-1", payload.Header.Get("Expires"))
	assert.Equal(t, "<html>", string(payload.Body))
	assert.False(t, payload.IsError())
}

func TestPayloadEmptyBody(t *testing.T) {
	buf := &bytes.Buffer{}
	assert.Nil(t, (&Payload{Status: 204, Reason: "No Content", Header: http.Header{}}).Write(buf))

	payload, err := ReadPayload(bytes.NewReader(buf.Bytes()), false)
	assert.Nil(t, err)
	assert.Equal(t, 204, payload.Status)
	assert.Equal(t, "", string(payload.Body))
}

func TestPayloadErrors(t *testing.T) {
	payload := PayloadFromError(errors.New("dial tcp: i/o timeout"))
	assert.Equal(t, ErrorStatus, payload.Status)
	assert.Equal(t, "dial tcp: i/o timeout", payload.Reason)
	assert.True(t, payload.IsError())
	assert.True(t, payload.IsNetworkError())

	// multiline errors are squashed, our format is line oriented
	payload = PayloadFromError(errors.New("one\ntwo"))
	buf := &bytes.Buffer{}
	assert.Nil(t, payload.Write(buf))
	got, err := ReadPayload(bytes.NewReader(buf.Bytes()), false)
	assert.Nil(t, err)
	assert.Equal(t, "one two", got.Reason)

	// http errors are errors, but not network errors
	payload = &Payload{Status: 404, Header: http.Header{}}
	assert.True(t, payload.IsError())
	assert.False(t, payload.IsNetworkError())
}

func TestPayloadFromResponse(t *testing.T) {
	resp := &http.Response{
		Status:     "404 Not Found",
		StatusCode: 404,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(strings.NewReader("nope")),
	}

	payload, err := PayloadFromResponse(resp)
	assert.Nil(t, err)
	assert.Equal(t, 404, payload.Status)
	assert.Equal(t, "Not Found", payload.Reason)
	assert.Equal(t, "nope", string(payload.Body))

	// the response body is still readable
	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)
	assert.Equal(t, "nope", string(body))

	// missing reason phrase
	resp = &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}
	payload, err = PayloadFromResponse(resp)
	assert.Nil(t, err)
	assert.Equal(t, "Internal Server Error", payload.Reason)
}

func TestPayloadResponse(t *testing.T) {
	payload := &Payload{
		Status: 200,
		Reason: "OK",
		Header: http.Header{"Content-Type": []string{"text/plain"}},
		Body:   []byte("hello"),
	}

	req := MustRequest("GET", "http://a.com")
	resp := payload.Response(req)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "200 OK", resp.Status)
	assert.Equal(t, int64(5), resp.ContentLength)
	assert.Equal(t, req, resp.Request)
	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)
	assert.Equal(t, "hello", string(body))
}

func TestPayloadInvalid(t *testing.T) {
	invalid := []string{
		"",
		"# comment",
		"# comment\nnot a status line\n\n",
		"# comment\nHTTPDISK 200 OK\nbogus header\n\n",
		"# comment\nHTTPDISK 200 OK\nheader: value\n",
	}
	for _, data := range invalid {
		_, err := ReadPayload(strings.NewReader(data), false)
		assert.NotNil(t, err, "expected an error for %q", data)
	}
}
