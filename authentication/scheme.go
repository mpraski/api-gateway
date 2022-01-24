package authentication

import "net/http"

type Scheme interface {
	Authenticate(*http.Request) error
}

const (
	authorizationHeader     = "Authorization"
	origAuthorizationHeader = "X-Orig-Authorization"
)

var sensitiveHeaders = []string{
	authorizationHeader,
	origAuthorizationHeader,
}

func ClearHeaders(r *http.Request) {
	for _, h := range sensitiveHeaders {
		r.Header.Del(h)
	}
}
