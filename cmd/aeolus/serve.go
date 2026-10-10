package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ChristopherGriffin/aeolus/agent"
	"github.com/ChristopherGriffin/aeolus/internal/api"
	"github.com/ChristopherGriffin/aeolus/internal/bundle"
	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/changelog"
	"github.com/ChristopherGriffin/aeolus/internal/conditions"
	"github.com/ChristopherGriffin/aeolus/internal/dhcpwatch"
	"github.com/ChristopherGriffin/aeolus/internal/feeds"
	"github.com/ChristopherGriffin/aeolus/internal/mcpadapter"
	"github.com/ChristopherGriffin/aeolus/internal/schema"
	"github.com/ChristopherGriffin/aeolus/internal/secret"
	"github.com/ChristopherGriffin/aeolus/internal/ui"
)

// runServe serves the API over HTTPS until SIGINT or SIGTERM.
func runServe(args []string, stderr io.Writer) error {
	srv, start, closeAll, err := newServer(args, stderr)
	if err != nil {
		return err
	}
	defer closeAll()
	// A long poll, such as an AP waiting on its keys (0070), ends when the
	// server stops: its request's context is the server's, cancelled as
	// Shutdown starts, which would otherwise wait it out and give up.
	base, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	srv.BaseContext = func(net.Listener) context.Context { return base }
	srv.RegisterOnShutdown(cancelBase)
	start()
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

// newServer opens everything serve needs and returns the configured server,
// a function that starts the DHCP listeners (0068), and one that closes it
// all.
func newServer(args []string, stderr io.Writer) (*http.Server, func(), func() error, error) {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	db := fs.String("db", "/var/lib/aeolus/aeolus.db", "change log database")
	keyPath := fs.String("key", "/etc/aeolus/secret.key", "secret key file (must exist; see aeolus init)")
	listen := fs.String("listen", ":8443", "address to listen on")
	certPath := fs.String("cert", "/etc/aeolus/tls.crt", "TLS certificate (a self-signed one is made if missing)")
	tlsKeyPath := fs.String("tls-key", "/etc/aeolus/tls.key", "TLS private key")
	hosts := fs.String("hosts", "", "comma-separated names and IPs for a self-signed certificate")
	condsPath := fs.String("conditions", "", "conditions database (default: conditions.db beside the change log)")
	keepDays := fs.Int("keep-state-days", defaultKeepDays(), "days to keep AP state reports (default from AEOLUS_KEEP_STATE_DAYS, else 30)")
	relayListen := fs.String("relay-listen", envOr("AEOLUS_RELAY_LISTEN", ":67"), "UDP address for relays' copies of DHCP requests, or off (0068)")
	knockListen := fs.String("knock-listen", envOr("AEOLUS_KNOCK_LISTEN", ":15002"), "TCP address for the option 224 listener, or off (0068)")
	feedDir := fs.String("feed-cache", envOr("AEOLUS_FEED_CACHE", ""), "directory for the cache of OpenWrt's feeds, or off (0069; default: feeds beside the change log)")
	feedMB := fs.Int("feed-cache-mb", envInt("AEOLUS_FEED_CACHE_MB", 2048), "the feed cache's size in MB (0069)")
	agentDir := fs.String("agent-bundles", envOr("AEOLUS_AGENT_BUNDLES", ""), "directory for the agent bundles APs update from, or off (0079; default: agent beside the change log)")
	if err := fs.Parse(args); err != nil {
		return nil, nil, nil, err
	}
	if *keepDays < 1 {
		return nil, nil, nil, errors.New("-keep-state-days must be at least 1")
	}
	if *condsPath == "" {
		*condsPath = filepath.Join(filepath.Dir(*db), "conditions.db")
	}
	if *feedDir == "" {
		*feedDir = filepath.Join(filepath.Dir(*db), "feeds")
	}
	if *feedMB < 1 {
		return nil, nil, nil, errors.New("-feed-cache-mb must be at least 1")
	}

	sch, err := schema.V1()
	if err != nil {
		return nil, nil, nil, err
	}
	box, err := secret.Load(*keyPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("secret key: %w", err)
	}
	if err := ensureCert(*certPath, *tlsKeyPath, splitList(*hosts), time.Now()); err != nil {
		return nil, nil, nil, err
	}
	cert, err := tls.LoadX509KeyPair(*certPath, *tlsKeyPath)
	if err != nil {
		return nil, nil, nil, err
	}
	// What a person checks when joining an AP by hand (0083).
	slog.Info("TLS certificate", "path", *certPath, "sha256", fingerprint(cert.Certificate[0]))
	log, err := changelog.Open(*db, changelog.Options{Check: api.Check(sch)})
	if err != nil {
		return nil, nil, nil, err
	}
	if log.Snapshot() == nil {
		log.Close()
		return nil, nil, nil, errors.New("the change log holds no Org yet; run aeolus init first")
	}
	// Every Org has Landing Zone and Sandbox (0032); the manager adds any
	// that are missing, in its own name (0036).
	if _, err := log.Commit(change.SystemActor, "built-in folders (0032)", change.Op{Kind: change.AddBuiltins}); err != nil && !errors.Is(err, change.ErrBuiltins) {
		log.Close()
		return nil, nil, nil, fmt.Errorf("adding built-in folders: %w", err)
	}
	// Each kind of AP adopted has a template (0085).
	api.NewKinds(log)
	conds, err := conditions.Open(*condsPath, nil)
	if err != nil {
		log.Close()
		return nil, nil, nil, fmt.Errorf("conditions: %w", err)
	}
	watch, err := dhcpwatch.Open(conds, nil)
	if err != nil {
		conds.Close()
		log.Close()
		return nil, nil, nil, fmt.Errorf("DHCP listeners: %w", err)
	}
	stopTrim := trimStates(conds, time.Duration(*keepDays)*24*time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	start := func() { listenDHCP(ctx, &wg, watch, *relayListen, *knockListen, cert) }
	closeAll := func() error {
		cancel()
		wg.Wait()
		stopTrim()
		return errors.Join(watch.Flush(), conds.Close(), log.Close())
	}
	slog.Info("aeolus loaded", "seq", log.Seq(), "keep_state_days", *keepDays)
	apiServer := api.New(log, sch, box, conds).WithWatch(watch)
	// The agent this build carries, kept with the releases before it, for
	// the APs to update themselves from (0079).
	if *agentDir != "off" {
		dir := *agentDir
		if dir == "" {
			dir = filepath.Join(filepath.Dir(*db), "agent")
		}
		store, own, err := keepAgent(dir, version, time.Now())
		if err != nil {
			slog.Error("agent bundles: APs won't update themselves", "err", err)
		} else {
			slog.Info("agent bundle", "release", own.Version, "hash", own.Hash, "dir", dir)
			apiServer = apiServer.WithAgents(store, own)
			// The same agent, for an AP installing from the manager (0083).
			if in, err := installKit(*certPath, version); err != nil {
				slog.Error("the installer: /install hands out nothing", "err", err)
			} else {
				apiServer = apiServer.WithInstall(in)
			}
		}
	}
	// Alerts go where the folders say, looked at every minute (0101).
	startListeners := start
	start = func() {
		startListeners()
		wg.Add(1)
		go func() {
			defer wg.Done()
			apiServer.Notify(ctx, time.Minute)
		}()
	}
	apiHandler := apiServer.Handler()
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpadapter.New(apiHandler, version))
	// APs fetch OpenWrt's packages through the manager (0069).
	if *feedDir != "off" {
		cache, err := feeds.New(*feedDir, feeds.Upstream, int64(*feedMB)<<20)
		if err != nil {
			closeAll()
			return nil, nil, nil, fmt.Errorf("feed cache: %w", err)
		}
		files, bytes := cache.Stats()
		slog.Info("feed cache", "dir", *feedDir, "files", files, "mb", bytes>>20, "max_mb", *feedMB)
		mux.Handle("/feeds/", cache)
	}
	// The UI answers browsers at / and serves its files; the API gets the rest (0042).
	mux.Handle("/", ui.Handler(apiHandler))
	return &http.Server{
		Addr:              *listen,
		Handler:           mux,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}, start, closeAll, nil
}

// listenDHCP starts the manager's DHCP listeners (0068), each unless its
// address is off, and the book's saving. A listener that cannot start is
// logged; the API serves without it.
func listenDHCP(ctx context.Context, wg *sync.WaitGroup, watch *dhcpwatch.Book, relay, knock string, cert tls.Certificate) {
	run := func(name string, f func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(); err != nil {
				slog.Error(name+" stopped", "err", err)
			}
		}()
	}
	run("saving what the DHCP listeners hear", func() error { watch.Keep(ctx); return nil })
	if relay != "off" {
		if pc, err := net.ListenPacket("udp4", relay); err != nil {
			slog.Error("the relay listener cannot start", "listen", relay, "err", err)
		} else {
			slog.Info("listening for relays' copies of DHCP requests", "listen", relay)
			run("the relay listener", func() error { return dhcpwatch.ListenRelay(ctx, pc, watch) })
		}
	}
	if knock != "off" {
		if ln, err := net.Listen("tcp", knock); err != nil {
			slog.Error("the option 224 listener cannot start", "listen", knock, "err", err)
		} else {
			slog.Info("listening for OpenWiFi APs' knocks", "listen", knock)
			run("the option 224 listener", func() error { return dhcpwatch.ServeKnocks(ctx, ln, cert, watch) })
		}
	}
}

// envInt is the environment variable name as a number, or def.
func envInt(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return n
	}
	return def
}

// envOr is the environment variable name, which serve.env can set, or def.
func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// defaultKeepDays reads AEOLUS_KEEP_STATE_DAYS, which serve.env can set
// (0039); 30 otherwise.
func defaultKeepDays() int {
	if n, err := strconv.Atoi(os.Getenv("AEOLUS_KEEP_STATE_DAYS")); err == nil {
		return n
	}
	return 30
}

// trimStates deletes state reports older than keep, now and every hour,
// until the returned function is called.
func trimStates(conds *conditions.Store, keep time.Duration) (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	trim := func() {
		if n, err := conds.TrimStates(keep); err != nil {
			slog.Error("trimming state reports", "err", err)
		} else if n > 0 {
			slog.Info("trimmed state reports", "deleted", n)
		}
	}
	go func() {
		defer close(finished)
		trim()
		tick := time.NewTicker(time.Hour)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				trim()
			case <-done:
				return
			}
		}
	}()
	return func() {
		close(done)
		<-finished
	}
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

// installKit is what the manager hands out at /install (0083): the
// installer, its own certificate, and LuCI's enrollment page.
func installKit(certPath, release string) (api.Install, error) {
	pem, err := os.ReadFile(certPath)
	if err != nil {
		return api.Install{}, err
	}
	luci, files, err := bundle.FromFSChecked(agent.LuCI, "luci", release, bundle.LuCIPathOK)
	if err != nil {
		return api.Install{}, err
	}
	return api.Install{Script: agent.WebInstall, CertPEM: pem, LuCI: luci, Files: files}, nil
}

// keepAgentReleases is how many releases' agents the manager keeps, for
// folders pinned to one (0079).
const keepAgentReleases = 10

// keepAgent makes the bundle of the agent this build carries, for its
// release, and keeps it in dir with the releases before it.
func keepAgent(dir, release string, now time.Time) (*bundle.Store, bundle.Bundle, error) {
	b, contents, err := bundle.FromFS(agent.Files, "files", release)
	if err != nil {
		return nil, bundle.Bundle{}, err
	}
	store, err := bundle.Open(dir, keepAgentReleases)
	if err != nil {
		return nil, bundle.Bundle{}, err
	}
	b, err = store.Put(b, contents, now)
	return store, b, err
}
