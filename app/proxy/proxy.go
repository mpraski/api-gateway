package proxy

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"github.com/mpraski/api-gateway/app/ratelimit"
	"github.com/mpraski/api-gateway/app/token"
	"golang.org/x/net/http/httpguts"
)

type Proxy struct {
	pool        *bytesPool
	routes      *routes
	tokens      *token.Client
	logger      *logging.Logger
	transport   *http.Transport
	rateLimiter ratelimit.HandleFunc
}

const (
	tokenLength = 2
	cookieName  = "blue-session"
)

var (
	welcomeMsg = []byte(`{"api": "BlueHealth"}`)
	// Hop-by-hop headers. These are removed when sent to the backend.
	// As of RFC 7230, hop-by-hop headers are required to appear in the
	// Connection header field. These are the headers defined by the
	// obsoleted RFC 2616 (section 13.5.1) and are used for backward
	// compatibility.
	hopHeaders = []string{
		"Connection",
		"Proxy-Connection", // non-standard but still sent by libcurl and rejected by e.g. google
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",      // canonicalized version of "TE"
		"Trailer", // not Trailers per URL above; https://www.rfc-editor.org/errata_search.php?eid=4522
		"Transfer-Encoding",
		"Upgrade",
	}
)

func New(configData string, tokens *token.Client, logger *logging.Logger, rateLimiter ratelimit.HandleFunc) (*Proxy, error) {
	routes, err := parseRoutes(configData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy routes: %w", err)
	}

	return &Proxy{
		pool:        newPool(),
		routes:      routes,
		tokens:      tokens,
		logger:      logger,
		transport:   newTransport(),
		rateLimiter: rateLimiter,
	}, nil
}

func (p *Proxy) Handler() http.Handler {
	return http.HandlerFunc(p.handle)
}

func (p *Proxy) handleRoot(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(welcomeMsg)

		return false
	}

	return true
}

func (p *Proxy) handleRateLimit(w http.ResponseWriter, r *http.Request, m match) bool {
	if p.rateLimiter == nil {
		return true
	}

	if !m.route.rateLimit.enabled {
		return true
	}

	return p.rateLimiter(w, r, ratelimit.Config{
		Limit:    m.route.rateLimit.limit,
		Duration: m.route.rateLimit.duration,
	})
}

func (p *Proxy) handleCors(w http.ResponseWriter, r *http.Request, m match) bool {
	if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
		if m.route.cors.enabled || m.route.cors.onlyPreflight {
			if m.route.cors.handlePreflight(w, r) {
				w.WriteHeader(http.StatusNoContent)
			} else {
				w.WriteHeader(http.StatusForbidden)
			}
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}

		return false
	}

	if m.route.cors.enabled {
		m.route.cors.handleActualRequest(w, r)
	}

	return true
}

func (p *Proxy) handleAuthorization(w http.ResponseWriter, r *http.Request, m match) bool {
	switch m.route.authz.policy {
	case custom, partner:
		return true

	case allowed:
		r.Header.Del("Authorization")
		return true

	case forbidden:
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return false

	case permitted, enforced:
		if m.route.authz.via != accessToken {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return false
		}

		var (
			t string
			o bool
		)

		switch m.route.authz.from {
		case header:
			t, o = tokenFromHeader(r)
		case cookie:
			t, o = tokenFromCookie(r)
		case nullFrom:
			break
		}

		r.Header.Del("Authorization")

		if !o {
			if m.route.authz.policy == permitted {
				return true
			}

			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return false
		}

		i, e := p.tokens.GetIdentity(r.Context(), t)
		if e != nil {
			if m.route.authz.policy == permitted {
				return true
			}

			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return false
		}

		r.Header.Set("Authorization", "Bearer "+i)

		return true

	case nullPolicy:
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return false
	}

	return false
}

func (p *Proxy) handleResponse(r *http.Response) {
	if r.StatusCode >= http.StatusInternalServerError {
		p.logger.Log(logging.Entry{
			Severity: logging.Error,
			Payload:  "upstream failed",
			HTTPRequest: &logging.HTTPRequest{
				Request:  r.Request,
				Status:   r.StatusCode,
				RemoteIP: r.Header.Get("X-Forwarded-For"),
			},
		})
	}
}

func (p *Proxy) modifyRequest(m match, req *http.Request) {
	var (
		targetScheme = m.route.target.Scheme
		targetQuery  = m.route.target.RawQuery
	)

	if targetScheme == "" {
		targetScheme = "http"
	}

	req.URL.Path = m.path
	req.URL.Host = m.route.target.Host
	req.URL.Scheme = targetScheme

	if targetQuery == "" || req.URL.RawQuery == "" {
		req.URL.RawQuery = targetQuery + req.URL.RawQuery
	} else {
		req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
	}

	if _, ok := req.Header["User-Agent"]; !ok {
		req.Header.Set("User-Agent", "")
	}
}

func (p *Proxy) getFlushInterval(res *http.Response) time.Duration {
	var (
		resCTHeader   = res.Header.Get("Content-Type")
		resCT, _, err = mime.ParseMediaType(resCTHeader)
	)

	// For Server-Sent Events responses, flush immediately.
	// The MIME type is defined in https://www.w3.org/TR/eventsource/#text-event-stream
	if err == nil && resCT == "text/event-stream" {
		return -1 // negative means immediately
	}

	// We might have the case of streaming for which Content-Length might be unset.
	if res.ContentLength == -1 {
		return -1
	}

	return 0
}

func (p *Proxy) copyResponse(dst io.Writer, src io.Reader, flushInterval time.Duration) error {
	if flushInterval != 0 {
		if wf, ok := dst.(writeFlusher); ok {
			mlw := &maxLatencyWriter{
				dst:     wf,
				latency: flushInterval,
			}

			defer mlw.stop()

			// set up initial timer so headers get flushed even if body writes are delayed
			mlw.flushPending = true
			mlw.t = time.AfterFunc(flushInterval, mlw.delayedFlush)

			dst = mlw
		}
	}

	b := p.pool.Get()
	defer p.pool.Put(b)

	_, err := p.copyBuffer(dst, src, *b)

	return err
}

// copyBuffer returns any write errors or non-EOF read errors, and the amount
// of bytes written.
func (p *Proxy) copyBuffer(dst io.Writer, src io.Reader, buf []byte) (int64, error) {
	var written int64

	for {
		nr, rerr := src.Read(buf)
		if rerr != nil && rerr != io.EOF && rerr != context.Canceled {
			p.logger.StandardLogger(logging.Error).Printf("httputil: Proxy read error during body copy: %v", rerr)
		}

		if nr > 0 {
			nw, werr := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}

			if werr != nil {
				return written, werr
			}

			if nr != nw {
				return written, io.ErrShortWrite
			}
		}

		if rerr != nil {
			if rerr == io.EOF {
				rerr = nil
			}

			return written, rerr
		}
	}
}

func (p *Proxy) handleUpgradeResponse(rw http.ResponseWriter, req *http.Request, res *http.Response) {
	var (
		reqUpType = upgradeType(req.Header)
		resUpType = upgradeType(res.Header)
	)

	if reqUpType != resUpType {
		p.logError(rw, req, fmt.Errorf("backend tried to switch protocol %q when %q was requested", resUpType, reqUpType))
		return
	}

	hj, ok := rw.(http.Hijacker)
	if !ok {
		p.logError(rw, req, fmt.Errorf("can't switch protocols using non-Hijacker ResponseWriter type %T", rw))
		return
	}

	backConn, ok := res.Body.(io.ReadWriteCloser)
	if !ok {
		p.logError(rw, req, fmt.Errorf("internal error: 101 switching protocols response with non-writable body"))
		return
	}

	backConnCloseCh := make(chan bool)

	go func() {
		// Ensure that the cancellation of a request closes the backend.
		// See issue https://golang.org/issue/35559.
		select {
		case <-req.Context().Done():
		case <-backConnCloseCh:
		}

		backConn.Close()
	}()

	defer close(backConnCloseCh)

	conn, brw, err := hj.Hijack()
	if err != nil {
		p.logError(rw, req, fmt.Errorf("hijack failed on protocol switch: %v", err))
		return
	}

	defer conn.Close()

	copyHeader(rw.Header(), res.Header)

	res.Header = rw.Header()
	res.Body = nil // so res.Write only writes the headers; we have res.Body in backConn above

	if err := res.Write(brw); err != nil {
		p.logError(rw, req, fmt.Errorf("response write: %v", err))
		return
	}

	if err := brw.Flush(); err != nil {
		p.logError(rw, req, fmt.Errorf("response flush: %v", err))
		return
	}

	var (
		errc = make(chan error, 1)
		spc  = switchProtocolCopier{user: conn, backend: backConn}
	)

	go spc.copyToBackend(errc)
	go spc.copyFromBackend(errc)

	<-errc
}

func (p *Proxy) handle(rw http.ResponseWriter, req *http.Request) {
	if !p.handleRoot(rw, req) {
		return
	}

	m, ok := p.routes.match(req.URL.Path)
	if !ok {
		http.Error(rw, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	if !p.handleRateLimit(rw, req, m) {
		return
	}

	if !p.handleCors(rw, req, m) {
		return
	}

	if !p.handleAuthorization(rw, req, m) {
		return
	}

	var (
		ctx    = req.Context()
		outreq = req.Clone(ctx)
	)

	if req.ContentLength == 0 {
		outreq.Body = nil // Issue 16036: nil Body for http.Transport retries
	}

	if outreq.Body != nil {
		// Reading from the request body after returning from a handler is not
		// allowed, and the RoundTrip goroutine that reads the Body can outlive
		// this handler. This can lead to a crash if the handler panics (see
		// Issue 46866). Although calling Close doesn't guarantee there isn't
		// any Read in flight after the handle returns, in practice it's safe to
		// read after closing it.
		defer outreq.Body.Close()
	}

	if outreq.Header == nil {
		outreq.Header = make(http.Header) // Issue 33142: historical behavior was to always allocate
	}

	p.modifyRequest(m, outreq)

	outreq.Close = false

	reqUpType := upgradeType(outreq.Header)

	removeConnectionHeaders(outreq.Header)

	// Remove hop-by-hop headers to the backend. Especially
	// important is "Connection" because we want a persistent
	// connection, regardless of what the client sent to us.
	for _, h := range hopHeaders {
		outreq.Header.Del(h)
	}

	// Issue 21096: tell backend applications that care about trailer support
	// that we support trailers. (We do, but we don't go out of our way to
	// advertise that unless the incoming client request thought it was worth
	// mentioning.) Note that we look at req.Header, not outreq.Header, since
	// the latter has passed through removeConnectionHeaders.
	if httpguts.HeaderValuesContainsToken(req.Header["Te"], "trailers") {
		outreq.Header.Set("Te", "trailers")
	}

	// After stripping all the hop-by-hop connection headers above, add back any
	// necessary for protocol upgrades, such as for websockets.
	if reqUpType != "" {
		outreq.Header.Set("Connection", "Upgrade")
		outreq.Header.Set("Upgrade", reqUpType)
	}

	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		// If we aren't the first proxy retain prior
		// X-Forwarded-For information as a comma+space
		// separated list and fold multiple headers into one.
		var (
			prior, ok = outreq.Header["X-Forwarded-For"]
			omit      = ok && prior == nil // Issue 38079: nil now means don't populate the header
		)

		if len(prior) > 0 {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}

		if !omit {
			outreq.Header.Set("X-Forwarded-For", clientIP)
		}
	}

	res, err := p.transport.RoundTrip(outreq)
	if err != nil {
		p.logError(rw, outreq, err)
		return
	}

	// Deal with 101 Switching Protocols responses: (WebSocket, h2c, etc)
	if res.StatusCode == http.StatusSwitchingProtocols {
		p.handleResponse(res)
		p.handleUpgradeResponse(rw, outreq, res)

		return
	}

	removeConnectionHeaders(res.Header)

	for _, h := range hopHeaders {
		res.Header.Del(h)
	}

	p.handleResponse(res)

	copyHeader(rw.Header(), res.Header)

	// The "Trailer" header isn't included in the Transport's response,
	// at least for *http.Transport. Build it up from Trailer.
	announcedTrailers := len(res.Trailer)
	if announcedTrailers > 0 {
		trailerKeys := make([]string, 0, len(res.Trailer))

		for k := range res.Trailer {
			trailerKeys = append(trailerKeys, k)
		}

		rw.Header().Add("Trailer", strings.Join(trailerKeys, ", "))
	}

	rw.WriteHeader(res.StatusCode)

	if err = p.copyResponse(rw, res.Body, p.getFlushInterval(res)); err != nil {
		defer res.Body.Close()

		p.logger.StandardLogger(logging.Error).Printf("aborting with incomplete response: %v", err)

		return
	}

	res.Body.Close() // close now, instead of defer, to populate res.Trailer

	if len(res.Trailer) > 0 {
		// Force chunking if we saw a response trailer.
		// This prevents net/http from calculating the length for short
		// bodies and adding a Content-Length.
		if fl, ok := rw.(http.Flusher); ok {
			fl.Flush()
		}
	}

	if len(res.Trailer) == announcedTrailers {
		copyHeader(rw.Header(), res.Trailer)
		return
	}

	for k, vv := range res.Trailer {
		k = http.TrailerPrefix + k

		for _, v := range vv {
			rw.Header().Add(k, v)
		}
	}
}

func (p *Proxy) logError(w http.ResponseWriter, r *http.Request, err error) {
	p.logger.Log(logging.Entry{
		Severity: logging.Error,
		Payload:  err.Error(),
		HTTPRequest: &logging.HTTPRequest{
			Request:  r,
			Status:   http.StatusBadGateway,
			RemoteIP: r.Header.Get("X-Forwarded-For"),
		},
	})

	w.WriteHeader(http.StatusBadGateway)
}
