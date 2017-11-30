package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"context"

	"github.com/pkg/errors"
	"github.com/rancher/authn-proxy/config"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
)

const (
	configPath = "/var/run/cattle.io/config/server.properties"
)

func NewReverseProxy(ctx context.Context) (http.Handler, error) {
	c := config.GetManager(ctx)

	cPath := os.Getenv("CONFIG_PATH")
	if cPath == "" {
		cPath = configPath
	}
	if err := c.AddConfigFile(cPath, config.PropertiesFile); err != nil {
		return nil, errors.Wrapf(err, "couldn't add config file %v", cPath)
	}

	backendScheme, backendHost, transport, err := getBackendConfig(c)
	if err != nil {
		return nil, errors.Wrap(err, "error determining backend")
	}
	logrus.Infof("Using backend scheme: %v, backendHost: %v", backendScheme, backendHost)

	director := func(req *http.Request) {
		req.URL.Scheme = backendScheme
		req.URL.Host = backendHost
	}

	reverseProxy := &httputil.ReverseProxy{
		Director:      director,
		FlushInterval: time.Millisecond * 100,
		Transport:     transport,
	}

	return reverseProxy, nil
}

func getBackendConfig(c *config.Manager) (string, string, http.RoundTripper, error) {
	scheme := c.Get("backend.scheme")
	host := c.Get("backend.host")
	caCertPath := ""
	if scheme == "" || host == "" {
		logrus.Infof("config properties backend.host or backend.scheme. Assuming in-cluster configuration")
		kubeConfig, err := rest.InClusterConfig()
		if err != nil {
			return "", "", nil, err
		}

		// For scheme and host
		u, err := url.Parse(kubeConfig.Host)
		if err != nil {
			return "", "", nil, errors.Wrap(err, "problem parsing kubeconfig url")
		}
		scheme = u.Scheme
		host = u.Host
		caCertPath = kubeConfig.CAFile
	}

	if caCertPath == "" && c.Get("backend.ca.cert.path") != "" {
		caCertPath = c.Get("backend.ca.cert.path")
	}

	if caCertPath == "" {
		return scheme, host, http.DefaultTransport, nil
	}

	caCert, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return "", "", nil, errors.Wrapf(err, "problem reading ca cert file %v", caCert)
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caCert)
	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: pool,
		},
	}
	return scheme, host, t, nil
}
