package feeds

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// upstream is a stand-in for OpenWrt's release tree, counting what it is
// asked for.
type upstream struct {
	srv   *httptest.Server
	hits  sync.Map // path -> *int32
	files map[string]string
	down  atomic.Bool
}

func newUpstream(t *testing.T, files map[string]string) *upstream {
	u := &upstream{files: files}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u.down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/releases/")
		n, _ := u.hits.LoadOrStore(p, new(int32))
		atomic.AddInt32(n.(*int32), 1)
		body, ok := u.files[p]
		if !ok {
			http.NotFound(w, r)
			return
		}
		time.Sleep(20 * time.Millisecond)
		io.WriteString(w, body)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) count(p string) int32 {
	n, ok := u.hits.Load(p)
	if !ok {
		return 0
	}
	return atomic.LoadInt32(n.(*int32))
}

const (
	idx = "25.12.5/targets/ipq806x/generic/packages/packages.adb"
	pkg = "25.12.5/packages/arm_cortex-a15_neon-vfpv4/base/usteer-2025.05.31~1.apk"
)

func get(t *testing.T, c *Cache, method, p string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	c.ServeHTTP(rec, httptest.NewRequest(method, p, nil))
	return rec.Code, rec.Body.String()
}

func open(t *testing.T, u *upstream, max int64) (*Cache, *time.Time) {
	t.Helper()
	c, err := New(t.TempDir(), u.srv.URL+"/releases/", max)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	c.now = func() time.Time { return clock }
	return c, &clock
}

func TestFetchedOnceThenServed(t *testing.T) {
	u := newUpstream(t, map[string]string{idx: "index v1", pkg: "usteer"})
	c, _ := open(t, u, 1<<20)
	for i := 0; i < 3; i++ {
		if code, body := get(t, c, "GET", "/feeds/"+pkg); code != 200 || body != "usteer" {
			t.Fatalf("get %d: %d %q", i, code, body)
		}
	}
	if n := u.count(pkg); n != 1 {
		t.Fatalf("upstream asked %d times", n)
	}
	if code, body := get(t, c, "HEAD", "/feeds/"+pkg); code != 200 || body != "" {
		t.Fatalf("head: %d %q", code, body)
	}
	if files, bytes := c.Stats(); files != 1 || bytes != int64(len("usteer")) {
		t.Fatalf("stats %d %d", files, bytes)
	}
}

func TestManyAskAtOnce(t *testing.T) {
	u := newUpstream(t, map[string]string{pkg: "usteer"})
	c, _ := open(t, u, 1<<20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, body := get(t, c, "GET", "/feeds/"+pkg); code != 200 || body != "usteer" {
				t.Errorf("%d %q", code, body)
			}
		}()
	}
	wg.Wait()
	if n := u.count(pkg); n != 1 {
		t.Fatalf("upstream asked %d times", n)
	}
}

func TestIndexesAreFetchedAgainButKeptWhenOffline(t *testing.T) {
	u := newUpstream(t, map[string]string{idx: "index v1"})
	c, clock := open(t, u, 1<<20)
	get(t, c, "GET", "/feeds/"+idx)
	u.files[idx] = "index v2"
	*clock = clock.Add(time.Hour)
	if _, body := get(t, c, "GET", "/feeds/"+idx); body != "index v1" {
		t.Fatalf("an hour on: %q", body)
	}
	*clock = clock.Add(IndexAge)
	if _, body := get(t, c, "GET", "/feeds/"+idx); body != "index v2" {
		t.Fatalf("a day on: %q", body)
	}
	// With the internet gone, the kept index is served, and what was never
	// fetched cannot be.
	u.down.Store(true)
	*clock = clock.Add(2 * IndexAge)
	if code, body := get(t, c, "GET", "/feeds/"+idx); code != 200 || body != "index v2" {
		t.Fatalf("offline: %d %q", code, body)
	}
	if code, _ := get(t, c, "GET", "/feeds/"+pkg); code != http.StatusBadGateway {
		t.Fatalf("offline, never fetched: %d", code)
	}
}

func TestOnlyOpenWrtsTree(t *testing.T) {
	u := newUpstream(t, map[string]string{})
	c, _ := open(t, u, 1<<20)
	for _, p := range []string{"/feeds/", "/feeds/../etc/passwd", "/feeds/25.12.5/../../x", "/feeds/.hidden/x", "/feeds/a//b",
		"/feeds/25.12.5/packages/", "/feeds/http:/evil.example/x", "/feeds/a%20b", "/feeds/%2e%2e/x"} {
		if code, _ := get(t, c, "GET", p); code != 404 {
			t.Errorf("%s: %d", p, code)
		}
	}
	if code, _ := get(t, c, "GET", "/feeds/25.12.5/nothing.apk"); code != 404 || u.count("25.12.5/nothing.apk") != 1 {
		t.Errorf("missing upstream: %d", code)
	}
	if code, _ := get(t, c, "POST", "/feeds/"+pkg); code != http.StatusMethodNotAllowed {
		t.Errorf("post: %d", code)
	}
}

func TestLeastRecentlyUsedGoesFirst(t *testing.T) {
	files := map[string]string{"a.apk": "aaaa", "b.apk": "bbbb", "c.apk": "cccc"}
	u := newUpstream(t, files)
	c, clock := open(t, u, 9)
	get(t, c, "GET", "/feeds/a.apk")
	*clock = clock.Add(time.Second)
	get(t, c, "GET", "/feeds/b.apk")
	*clock = clock.Add(time.Second)
	get(t, c, "GET", "/feeds/a.apk") // a is used again
	*clock = clock.Add(time.Second)
	get(t, c, "GET", "/feeds/c.apk") // over 9 bytes: b goes
	if files, bytes := c.Stats(); files != 2 || bytes != 8 {
		t.Fatalf("stats %d %d", files, bytes)
	}
	get(t, c, "GET", "/feeds/a.apk")
	get(t, c, "GET", "/feeds/b.apk")
	if u.count("a.apk") != 1 || u.count("b.apk") != 2 {
		t.Fatalf("a %d, b %d", u.count("a.apk"), u.count("b.apk"))
	}
}

func TestReopened(t *testing.T) {
	u := newUpstream(t, map[string]string{pkg: "usteer"})
	dir := t.TempDir()
	c, _ := New(dir, u.srv.URL+"/releases/", 1<<20)
	get(t, c, "GET", "/feeds/"+pkg)
	c2, err := New(dir, u.srv.URL+"/releases/", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if files, _ := c2.Stats(); files != 1 {
		t.Fatalf("reopened with %d files", files)
	}
	if _, body := get(t, c2, "GET", "/feeds/"+pkg); body != "usteer" || u.count(pkg) != 1 {
		t.Fatalf("reopened: %q, upstream %d", body, u.count(pkg))
	}
}

// An upstream that takes the connection and never answers is given up on
// soon, and a kept copy is served (0069): offline, connections often go
// unanswered rather than refused.
func TestAnUpstreamThatNeverAnswers(t *testing.T) {
	hang := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-hang:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(hang)
	c, err := New(t.TempDir(), srv.URL+"/releases/", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	c.client, c.answerBy = client(time.Second, 200*time.Millisecond), time.Second
	start := time.Now()
	if code, _ := get(t, c, "GET", "/feeds/"+pkg); code != http.StatusBadGateway {
		t.Fatalf("never fetched: %d", code)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
}

// A proxy that takes the connection and never answers the CONNECT is given
// up on as soon: the transport's own limits don't cover the tunnel.
func TestAProxyThatNeverAnswers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close() // held open, never answered
		}
	}()
	c, err := New(t.TempDir(), "https://downloads.invalid/releases/", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	c.client, c.answerBy = client(time.Second, time.Second), 300*time.Millisecond
	c.client.Transport.(*http.Transport).Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: ln.Addr().String()})
	start := time.Now()
	if code, _ := get(t, c, "GET", "/feeds/"+pkg); code != http.StatusBadGateway {
		t.Fatalf("through a silent proxy: %d", code)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
}
