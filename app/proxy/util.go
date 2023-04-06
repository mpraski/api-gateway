package proxy

import (
	"io"
	"net/http"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http/httpguts"
)

type (
	writeFlusher interface {
		io.Writer
		http.Flusher
	}

	maxLatencyWriter struct {
		dst     writeFlusher
		latency time.Duration // non-zero; negative means to flush immediately

		mu           sync.Mutex // protects t, flushPending, and dst.Flush
		t            *time.Timer
		flushPending bool
	}

	switchProtocolCopier struct {
		user, backend io.ReadWriter
	}
)

const toLower = 'a' - 'A'

func parseHeaderList(headerList string) []string {
	var (
		t     = 0
		l     = len(headerList)
		h     = make([]byte, 0, l)
		upper = true
	)

	for i := 0; i < l; i++ {
		if headerList[i] == ',' {
			t++
		}
	}

	headers := make([]string, 0, t)

	for i := 0; i < l; i++ {
		b := headerList[i]

		switch {
		case b >= 'a' && b <= 'z':
			if upper {
				h = append(h, b-toLower)
			} else {
				h = append(h, b)
			}

		case b >= 'A' && b <= 'Z':
			if !upper {
				h = append(h, b+toLower)
			} else {
				h = append(h, b)
			}

		case b == '-' || b == '_' || b == '.' || (b >= '0' && b <= '9'):
			h = append(h, b)
		}

		if b == ' ' || b == ',' || i == l-1 {
			if len(h) > 0 {
				headers = append(headers, string(h))
				h = h[:0]
				upper = true
			}
		} else {
			upper = b == '-' || b == '_'
		}
	}

	return headers
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func tokenFromHeader(r *http.Request) (value string, found bool) {
	if arr := strings.Split(r.Header.Get("Authorization"), " "); len(arr) == tokenLength {
		value = arr[1]
		found = true
	}

	return
}

func tokenFromCookie(r *http.Request) (value string, found bool) {
	if c, err := r.Cookie(cookieName); err == nil {
		value = strings.TrimPrefix(c.Value, cookieName+"=")

		if value != "" {
			found = true
		}
	}

	return
}

// removeConnectionHeaders removes hop-by-hop headers listed in the "Connection" header of h.
// See RFC 7230, section 6.1
func removeConnectionHeaders(h http.Header) {
	for _, f := range h["Connection"] {
		for _, sf := range strings.Split(f, ",") {
			if sf = textproto.TrimString(sf); sf != "" {
				h.Del(sf)
			}
		}
	}
}

func upgradeType(h http.Header) string {
	if !httpguts.HeaderValuesContainsToken(h["Connection"], "Upgrade") {
		return ""
	}

	return strings.ToLower(h.Get("Upgrade"))
}

func (m *maxLatencyWriter) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	n, err = m.dst.Write(p)

	if m.latency < 0 {
		m.dst.Flush()
		return
	}

	if m.flushPending {
		return
	}

	if m.t == nil {
		m.t = time.AfterFunc(m.latency, m.delayedFlush)
	} else {
		m.t.Reset(m.latency)
	}

	m.flushPending = true

	return
}

func (m *maxLatencyWriter) delayedFlush() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.flushPending { // if stop was called but AfterFunc already started this goroutine
		return
	}

	m.dst.Flush()

	m.flushPending = false
}

func (m *maxLatencyWriter) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.flushPending = false

	if m.t != nil {
		m.t.Stop()
	}
}

func (c switchProtocolCopier) copyFromBackend(errc chan<- error) {
	_, err := io.Copy(c.user, c.backend)
	errc <- err
}

func (c switchProtocolCopier) copyToBackend(errc chan<- error) {
	_, err := io.Copy(c.backend, c.user)
	errc <- err
}
