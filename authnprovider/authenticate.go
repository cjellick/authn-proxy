package authnprovider

import "net/http"

type Authenticator interface {
	Authenticate(req *http.Request) (authed bool, user string, groups []string, err error)
}

func NewAuthnProvider() Authenticator {
	return &hackAuthn{}
}
