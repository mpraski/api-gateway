package authentication

import "net/http"

type Scheme interface {
	Authenticate(*http.Request) error
}
