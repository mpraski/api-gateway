package proxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

const (
	DefaultMaxIdleConns          = 100
	DefaultDialTimeout           = 30 * time.Second
	DefaultKeepalive             = 30 * time.Second
	DefaultTLSHandshakeTimeout   = 10 * time.Second
	DefaultExpectContinueTimeout = time.Second
	DefaultResponseHeaderTimeout = 30 * time.Second
	DefaultIdleConnsPerHost      = 64
	DefaultIdleConnTimeout       = 90 * time.Second
)

func newTransport() *http.Transport {
	t := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   DefaultDialTimeout,
			KeepAlive: DefaultKeepalive,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          DefaultMaxIdleConns,
		IdleConnTimeout:       DefaultIdleConnTimeout,
		TLSHandshakeTimeout:   DefaultTLSHandshakeTimeout,
		ExpectContinueTimeout: DefaultExpectContinueTimeout,
		ResponseHeaderTimeout: DefaultResponseHeaderTimeout,
		MaxIdleConnsPerHost:   DefaultIdleConnsPerHost,
		//nolint:gosec //not relevant
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	ticker := time.NewTicker(time.Minute)

	go func(transport *http.Transport) {
		for range ticker.C {
			transport.DisableKeepAlives = true
			transport.CloseIdleConnections()
			transport.DisableKeepAlives = false
		}
	}(t)

	return t
}
