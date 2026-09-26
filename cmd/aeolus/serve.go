package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ChristopherGriffin/aeolus/internal/api"
	"github.com/ChristopherGriffin/aeolus/internal/changelog"
	"github.com/ChristopherGriffin/aeolus/internal/mcpadapter"
	"github.com/ChristopherGriffin/aeolus/internal/schema"
	"github.com/ChristopherGriffin/aeolus/internal/secret"
)

// runServe serves the API over HTTPS until SIGINT or SIGTERM.
func runServe(args []string, stderr io.Writer) error {
	srv, closeLog, err := newServer(args, stderr)
	if err != nil {
		return err
	}
	defer closeLog()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServeTLS("", "") }()
	slog.Info("aeolus serving", "version", version, "listen", srv.Addr)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	slog.Info("aeolus stopping")
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// newServer opens everything serve needs and returns the configured server
// and a function that closes the change log.
func newServer(args []string, stderr io.Writer) (*http.Server, func() error, error) {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	db := fs.String("db", "/var/lib/aeolus/aeolus.db", "change log database")
	keyPath := fs.String("key", "/etc/aeolus/secret.key", "secret key file (must exist; see aeolus init)")
	listen := fs.String("listen", ":8443", "address to listen on")
	certPath := fs.String("cert", "/etc/aeolus/tls.crt", "TLS certificate (a self-signed one is made if missing)")
	tlsKeyPath := fs.String("tls-key", "/etc/aeolus/tls.key", "TLS private key")
	hosts := fs.String("hosts", "", "comma-separated names and IPs for a self-signed certificate")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}

	sch, err := schema.V1()
	if err != nil {
		return nil, nil, err
	}
	box, err := secret.Load(*keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("secret key: %w", err)
	}
	if err := ensureCert(*certPath, *tlsKeyPath, splitList(*hosts), time.Now()); err != nil {
		return nil, nil, err
	}
	cert, err := tls.LoadX509KeyPair(*certPath, *tlsKeyPath)
	if err != nil {
		return nil, nil, err
	}
	log, err := changelog.Open(*db, changelog.Options{Check: api.Check(sch)})
	if err != nil {
		return nil, nil, err
	}
	if log.Snapshot() == nil {
		log.Close()
		return nil, nil, errors.New("the change log holds no Org yet; run aeolus init first")
	}
	slog.Info("aeolus loaded", "seq", log.Seq())
	apiHandler := api.New(log, sch, box).Handler()
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpadapter.New(apiHandler, version))
	mux.Handle("/", apiHandler)
	return &http.Server{
		Addr:              *listen,
		Handler:           mux,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}, log.Close, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
