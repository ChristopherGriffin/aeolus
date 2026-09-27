package ui

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func serveUI(t *testing.T, method, path, accept string) *http.Response {
	t.Helper()
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "api")
	})
	req := httptest.NewRequest(method, path, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	Handler(api).ServeHTTP(rec, req)
	return rec.Result()
}

func body(t *testing.T, r *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBrowsersGetThePageAndClientsGetTheAPI(t *testing.T) {
	r := serveUI(t, "GET", "/", "text/html,application/xhtml+xml")
	if r.StatusCode != 200 || !strings.Contains(body(t, r), `src="/ui/app.js"`) {
		t.Fatalf("page: %d", r.StatusCode)
	}
	if csp := r.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP = %q", csp)
	}
	for _, h := range []string{"X-Content-Type-Options", "Referrer-Policy", "X-Frame-Options"} {
		if r.Header.Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
	for _, c := range []struct{ path, accept string }{{"/", "application/json"}, {"/", ""}, {"/v1/whoami", "text/html"}, {"/healthz", ""}} {
		if got := body(t, serveUI(t, "GET", c.path, c.accept)); got != "api" {
			t.Errorf("%s (%s) went to the UI: %q", c.path, c.accept, got)
		}
	}
}

func TestFiles(t *testing.T) {
	for path, typ := range map[string]string{
		"/ui/app.js":        "text/javascript",
		"/ui/views/tree.js": "text/javascript",
		"/ui/app.css":       "text/css",
	} {
		r := serveUI(t, "GET", path, "")
		if r.StatusCode != 200 || !strings.HasPrefix(r.Header.Get("Content-Type"), typ) || r.Header.Get("Content-Security-Policy") == "" {
			t.Errorf("%s: %d %q", path, r.StatusCode, r.Header.Get("Content-Type"))
		}
	}
	for _, path := range []string{"/ui/", "/ui/views", "/ui/views/", "/ui/nope.js", "/ui/../ui.go", "/ui/%2e%2e/ui.go"} {
		if r := serveUI(t, "GET", path, ""); r.StatusCode != 404 {
			t.Errorf("%s: %d, want 404", path, r.StatusCode)
		}
	}
	if r := serveUI(t, "POST", "/ui/app.js", ""); r.StatusCode != 405 {
		t.Errorf("POST: %d", r.StatusCode)
	}
}

// Every module the page imports is shipped, and the page builds content
// only as text (0042): no innerHTML, no eval.
func TestModules(t *testing.T) {
	importRE := regexp.MustCompile(`from '(\.{1,2}/[^']+)'`)
	err := fs.WalkDir(static, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".js") {
			return err
		}
		src, err := fs.ReadFile(static, path)
		if err != nil {
			return err
		}
		for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "eval(", "new Function"} {
			if strings.Contains(string(src), bad) {
				t.Errorf("%s uses %s", path, bad)
			}
		}
		dir := path[:strings.LastIndex(path, "/")]
		for _, m := range importRE.FindAllStringSubmatch(string(src), -1) {
			target := clean(dir + "/" + m[1])
			if _, err := fs.Stat(static, target); err != nil {
				t.Errorf("%s imports %s, which is not shipped", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func clean(p string) string {
	var out []string
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case ".", "":
		case "..":
			out = out[:len(out)-1]
		default:
			out = append(out, seg)
		}
	}
	return strings.Join(out, "/")
}
