package main

import (
	"net/http"
	"os"

	"context"

	"github.com/rancher/authn-proxy/config"
	"github.com/rancher/authn-proxy/impersonation"
	"github.com/rancher/authn-proxy/proxy"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Action = run
	app.Run(os.Args)
}

func run() {
	logrus.Infof("Configuring...")

	ctx, cancelF := context.WithCancel(context.Background())
	defer cancelF()

	p, err := proxy.NewReverseProxy(ctx)
	if err != nil {
		logrus.Fatalf("Failed to get reverse proxy: %v", err)
	}

	handler, err := impersonation.NewAuthnHeaderHandler(ctx, p)
	if err != nil {
		logrus.Fatalf("Failed to get impersonation handler: %v", err)
	}

	conf := config.GetManager(ctx)

	if conf.Get("log.level") != "" {
		l, err := logrus.ParseLevel(c.Get("log.level"))
		if err == nil {
			logrus.SetLevel(l)
		}
	}

	httpsHost := conf.Get("frontend.https.host")
	if httpsHost != "" {
		go func() {
			logrus.Infof("Starting https server listening on %v. Backend %v", httpsHost)
			err := http.ListenAndServeTLS(httpsHost, conf.Get("frontend.ssl.cert.path"), conf.Get("frontend.ssl.key.path"), handler)
			logrus.Fatalf("https server exited. Error: %v", err)
		}()
	}

	httpHost := conf.Get("frontend.http.host")
	server := &http.Server{
		Handler: handler,
		Addr:    httpHost,
	}
	logrus.Infof("Starting http server listening on %v.", httpHost)
	err = server.ListenAndServe()
	logrus.Infof("https server exited. Error: %v", err)
}
