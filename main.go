package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"k8s.io/client-go/rest"
)

func main() {

	app := cli.NewApp()
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "backend-addr",
			Usage: "Address (host:port) of the server to proxy to",
		},
		cli.StringFlag{
			Name:  "backend-scheme",
			Usage: "Scheme (http or https) of the server to proxy to",
			Value: "https",
		},
		cli.StringFlag{
			Name:  "frontend-http-addr",
			Usage: "Address (host:port) to listen on",
		},
		cli.StringFlag{
			Name:  "frontend-https-addr",
			Usage: "Address (host:port) to listen on",
		},
		cli.StringFlag{
			Name:  "frontend-ssl-cert-path",
			Usage: "SSL Cert for securing frontend https",
		},
		cli.StringFlag{
			Name:  "frontend-ssl-key-path",
			Usage: "SSL key for securing frontend https",
		},
		cli.StringFlag{
			Name:  "ca-cert-path",
			Usage: "Path to ca cert",
		},
		cli.StringFlag{
			Name:  "token-path",
			Usage: "Path to service account token",
		},
	}

	app.Action = run
	app.Run(os.Args)
}

func run(c *cli.Context) {
	logrus.Infof("Launching...")
	conf, err := buildConfig(c)
	if err != nil {
		logrus.Fatal(err)
		return
	}

	handler, err := newProxyHandler(conf)
	if err != nil {
		logrus.Fatal(err, "problem building proxy handler")
		return
	}

	server := &http.Server{
		Handler: handler,
		Addr:    conf.frontendHTTPAddr,
	}

	if conf.frontendHTTPSAddr != "" {
		go func() {
			logrus.Infof("Starting https server listening on %v. Backend %v", conf.frontendHTTPSAddr, conf.backendHost)
			err := http.ListenAndServeTLS(conf.frontendHTTPSAddr, conf.frontendSSLCert, conf.frontendSSLKey, handler)
			logrus.Fatalf("https server exited. Error: %v", err)
		}()
	}

	logrus.Infof("Starting http server listening on %v. Backend %v", conf.frontendHTTPAddr, conf.backendHost)
	err = server.ListenAndServe()
	logrus.Infof("https server exited. Error: %v", err)
}

func newProxyHandler(conf *config) (http.Handler, error) {
	director := func(req *http.Request) {
		req.URL.Scheme = conf.backendScheme
		req.URL.Host = conf.backendHost
	}

	reverseProxy := &httputil.ReverseProxy{
		Director:      director,
		FlushInterval: time.Millisecond * 100,
		Transport:     conf.transport,
	}

	ph := &proxyHandler{
		reverseProxy: reverseProxy,
		serverAddr:   conf.backendHost,
		token:        conf.token,
		auth:         &hackAuthn{},
	}

	return ph, nil
}

type Authenticator interface {
	Authenticate(req *http.Request) (authed bool, user string, groups []string, err error)
}

type proxyHandler struct {
	reverseProxy http.Handler
	serverAddr   string
	token        string
	auth         Authenticator
}

func (h *proxyHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	authed, user, groups, err := h.auth.Authenticate(req)
	if err != nil {
		logrus.Errorf("Error encountered while authenticating: %v", err)
		http.Error(rw, "The server encountered a problem", 500)
		return
	}

	if !authed {
		http.Error(rw, "Failed authentication", 401)
	}

	logrus.Infof("Impersonating user %v, groups %v", user, groups)

	req.Header.Set("Impersonate-User", user)

	req.Header.Del("Impersonate-Group")
	for _, group := range groups {
		req.Header.Add("Impersonate-Group", group)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", h.token))

	if len(req.Header.Get("Upgrade")) > 0 {
		h.serveWebsocket(rw, req)
	} else {
		h.reverseProxy.ServeHTTP(rw, req)
	}
}

func (h *proxyHandler) serveWebsocket(rw http.ResponseWriter, req *http.Request) {
	// Inspired by https://groups.google.com/forum/#!searchin/golang-nuts/httputil.ReverseProxy$20$2B$20websockets/golang-nuts/KBx9pDlvFOc/01vn1qUyVdwJ
	target := h.serverAddr
	d, err := net.Dial("tcp", target)
	if err != nil {
		logrus.WithField("error", err).Error("Error dialing websocket backend.")
		http.Error(rw, "Unable to establish websocket connection: can't dial.", 500)
		return
	}
	hj, ok := rw.(http.Hijacker)
	if !ok {
		http.Error(rw, "Unable to establish websocket connection: no hijacker.", 500)
		return
	}
	nc, _, err := hj.Hijack()
	if err != nil {
		logrus.WithField("error", err).Error("Hijack error.")
		http.Error(rw, "Unable to establish websocket connection: can't hijack.", 500)
		return
	}
	defer nc.Close()
	defer d.Close()

	err = req.Write(d)
	if err != nil {
		logrus.WithField("error", err).Error("Error copying request to target.")
		return
	}

	errChannel := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errChannel <- err
	}
	go cp(d, nc)
	go cp(nc, d)
	<-errChannel
}

type config struct {
	frontendHTTPAddr  string
	frontendHTTPSAddr string
	frontendSSLCert   string
	frontendSSLKey    string
	token             string
	backendHost       string
	backendScheme     string
	transport         http.RoundTripper
	kubeConfig        *rest.Config
}

func buildConfig(c *cli.Context) (*config, error) {
	backendAddr := c.String("backend-addr")
	backendScheme := c.String("backend-scheme")
	caCertPath := c.String("ca-cert-path")
	tokenPath := c.String("token-path")
	frontendHTTPAddr := c.String("frontend-http-addr")
	frontendHTTPSAddr := c.String("frontend-https-addr")
	frontendKeyPath := c.String("frontend-ssl-key-path")
	frontendCertPath := c.String("frontend-ssl-cert-path")

	if frontendHTTPAddr == "" {
		return nil, errors.New("frontend-addr not provided")
	}

	conf := &config{
		frontendHTTPAddr:  frontendHTTPAddr,
		frontendHTTPSAddr: frontendHTTPSAddr,
		frontendSSLCert:   frontendCertPath,
		frontendSSLKey:    frontendKeyPath,
	}

	if tokenPath == "" || backendAddr == "" || backendScheme == "" {
		logrus.Debugf("args backend-host backend-scheme token-path not found. Assuming in-cluster configuration")
		kubeConfig, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}

		// For scheme and host
		u, err := url.Parse(kubeConfig.Host)
		if err != nil {
			return nil, errors.Wrap(err, "problem parsing kubeconfig url")
		}

		conf.backendScheme = u.Scheme
		conf.backendHost = u.Host
		conf.token = kubeConfig.BearerToken
		conf.transport = kubeConfig.Transport
		conf.kubeConfig = kubeConfig
	} else {
		// Scheme and host
		conf.backendHost = backendAddr
		conf.backendScheme = backendScheme

		// Token
		bytes, err := ioutil.ReadFile(tokenPath)
		if err != nil {
			return nil, errors.Wrapf(err, "problem reading token file %v", tokenPath)
		}
		conf.token = strings.TrimSpace(string(bytes))

		// Transport
		if caCertPath != "" {
			caCert, err := ioutil.ReadFile(caCertPath)
			if err != nil {
				return nil, errors.Wrapf(err, "problem reading ca cert file %v", caCertPath)
			}
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(caCert)
			conf.transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: pool,
				},
			}
		} else {
			conf.transport = http.DefaultTransport
		}
	}

	return conf, nil
}
