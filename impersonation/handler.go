package impersonation

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/pkg/errors"
	"github.com/rancher/authn-proxy/authnprovider"
	"github.com/rancher/authn-proxy/config"
	"github.com/sirupsen/logrus"
)

const (
	tokenPath  = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	configPath = "/var/run/config/cattle.io/config"
)

func NewAuthnHeaderHandler(ctx context.Context, next http.Handler) (http.Handler, error) {
	c := config.GetManager(ctx)

	tPath := os.Getenv("TOKEN_PATH")
	if tPath == "" {
		tPath = tokenPath
	}
	if err := c.AddConfigFile(tPath, config.SingleValueFile); err != nil {
		return nil, errors.Wrapf(err, "couldn't add token config file %v", tPath)
	}

	cPath := os.Getenv("CONFIG_PATH")
	if cPath == "" {
		cPath = configPath
	}
	if err := c.AddConfigFile(cPath, config.PropertiesFile); err != nil {
		return nil, errors.Wrapf(err, "couldn't add config file %v", cPath)
	}

	auth := authnprovider.NewAuthnProvider()

	return &authHeaderHandler{
		auth:   auth,
		config: c,
		next:   next,
	}, nil
}

type authHeaderHandler struct {
	auth   authnprovider.Authenticator
	next   http.Handler
	config *config.Manager
}

func (h authHeaderHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	authed, user, groups, err := h.auth.Authenticate(req)
	if err != nil {
		logrus.Errorf("Error encountered while authenticating: %v", err)
		// TODO who will handle standardizing the format of 400/500 response bodies?
		http.Error(rw, "The server encountered a problem", 500)
		return
	}

	if !authed {
		http.Error(rw, "Failed authentication", 401)
	}

	logrus.Debugf("Impersonating user %v, groups %v", user, groups)

	req.Header.Set("Impersonate-User", user)

	req.Header.Del("Impersonate-Group")
	for _, group := range groups {
		req.Header.Add("Impersonate-Group", group)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", h.config.Get("token")))

	h.next.ServeHTTP(rw, req)
}
