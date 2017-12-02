package authnprovider

import (
	"encoding/base64"
	"net/http"
	"strings"
)

type hackAuthn struct{}

func (a *hackAuthn) Authenticate(req *http.Request) (bool, string, []string, error) {
	user, groupsIMeanPassword, ok := req.BasicAuth()
	if ok {
		groups := strings.Split(groupsIMeanPassword, ",")
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

	parts := strings.SplitN(string(bytes), ":", 2)
	user = parts[0]
	groups := []string{}
	if len(parts) == 2 {
		groups = strings.Split(parts[1], ",")
	}
	return true, user, groups, nil
}
