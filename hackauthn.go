package main

import (
	"encoding/base64"
	"net/http"
	"strings"
)

type hackAuthn struct {
}

// TODO Agree on the token/cookie that vince will pass
func (a *hackAuthn) Authenticate(req *http.Request) (bool, string, []string, error) {
	user, groupsIMeanPassword, ok := req.BasicAuth()
	if ok {
		groups := strings.Split(groupsIMeanPassword, ":")
		return true, user, groups, nil
	}

	authCookie, err := req.Cookie("Authentication")
	if err != nil {
		if err == http.ErrNoCookie {
			return false, "", nil, nil
		}
		return false, "", nil, err
	}

	bytes, err := base64.StdEncoding.DecodeString(authCookie.Value)
	if err != nil {
		return false, "", nil, err
	}

	parts := strings.Split(string(bytes), ":")
	return true, parts[0], parts[1:], nil
}
