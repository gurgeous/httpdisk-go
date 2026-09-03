package httpdisk

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ErrorStatus is the fake http status used for cached network errors, matching
// the ruby httpdisk gem.
const ErrorStatus = 999

// Payload is a cached response, in the same on disk format used by the ruby
// httpdisk gem:
//
//	# GET http://example.com/
//	HTTPDISK 200 OK
//	content-type: text/html
//
//	<body>
type Payload struct {
	Comment string
	Status  int
	Reason  string
	Header  http.Header
	Body    []byte
}

var statusLineRe = regexp.MustCompile(`^HTTPDISK (\d+) ?(.*)$`)

// ReadPayload parses a payload. If peek is true the body is skipped, which is
// handy when all we want is the status.
func ReadPayload(r io.Reader, peek bool) (*Payload, error) {
	payload := &Payload{Header: http.Header{}}
	buf := bufio.NewReader(r)

	// comment
	line, err := readLine(buf)
	if err != nil {
		return nil, err
	}
	payload.Comment = strings.TrimPrefix(line, "# ")

	// status line
	if line, err = readLine(buf); err != nil {
		return nil, err
	}
	m := statusLineRe.FindStringSubmatch(line)
	if m == nil {
		return nil, fmt.Errorf("invalid status line %q", line)
	}
	if payload.Status, err = strconv.Atoi(m[1]); err != nil {
		return nil, err
	}
	payload.Reason = m[2]

	// headers, terminated by a blank line
	for {
		if line, err = readLine(buf); err != nil {
			return nil, err
		}
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			return nil, fmt.Errorf("invalid header %q", line)
		}
		payload.Header.Add(key, value)
	}

	// body
	if !peek {
		if payload.Body, err = io.ReadAll(buf); err != nil {
			return nil, err
		}
	}

	return payload, nil
}

// PayloadFromResponse creates a Payload from a response. The response body is
// drained and replaced so it can still be read by the caller.
func PayloadFromResponse(resp *http.Response) (*Payload, error) {
	// always close the original body, even if the read failed
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	return &Payload{
		Status: resp.StatusCode,
		Reason: reasonPhrase(resp),
		Header: resp.Header,
		Body:   body,
	}, nil
}

// PayloadFromError creates a 999 Payload for a network error.
func PayloadFromError(err error) *Payload {
	return &Payload{
		Status: ErrorStatus,
		Reason: oneline(err.Error()),
		Header: http.Header{},
	}
}

// Response turns this payload back into an http.Response.
func (payload *Payload) Response(req *http.Request) *http.Response {
	return &http.Response{
		Status:        strings.TrimSpace(fmt.Sprintf("%d %s", payload.Status, payload.Reason)),
		StatusCode:    payload.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        payload.Header,
		Body:          io.NopCloser(bytes.NewReader(payload.Body)),
		ContentLength: int64(len(payload.Body)),
		Request:       req,
	}
}

func (payload *Payload) Write(w io.Writer) error {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "# %s\n", oneline(payload.Comment))
	fmt.Fprintf(buf, "HTTPDISK %d %s\n", payload.Status, oneline(payload.Reason))

	keys := make([]string, 0, len(payload.Header))
	for key := range payload.Header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range payload.Header[key] {
			fmt.Fprintf(buf, "%s: %s\n", key, oneline(value))
		}
	}

	buf.WriteString("\n")
	buf.Write(payload.Body)

	_, err := w.Write(buf.Bytes())
	return err
}

// one-liners
func (payload *Payload) IsError() bool        { return payload.Status >= 400 }
func (payload *Payload) IsNetworkError() bool { return payload.Status == ErrorStatus }

//
// helpers
//

func readLine(buf *bufio.Reader) (string, error) {
	line, err := buf.ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// The reason phrase from a response, ex "OK" from "200 OK"
func reasonPhrase(resp *http.Response) string {
	if _, reason, ok := strings.Cut(resp.Status, " "); ok && reason != "" {
		return reason
	}
	return http.StatusText(resp.StatusCode)
}

// Our format is line oriented, so newlines have to go
func oneline(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}
