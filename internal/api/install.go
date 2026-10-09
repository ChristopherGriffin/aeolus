package api

import (
	"bytes"
	"errors"
	"io/fs"
	"net/http"
	"regexp"

	"github.com/ChristopherGriffin/aeolus/internal/bundle"
)

// Installing from the manager (0083): an AP with nothing of Aeolus gets its
// agent from the manager it will join, with one command,
//
//	wget -qO- --no-check-certificate https://<manager>:8443/install | sh
//
// These routes need no token, as enrollment needs none (0033): what they
// hand out is the manager's release of the agent, its LuCI page, and the
// manager's own certificate, none of it secret. The script checks every
// file it fetches against the manifest's SHA-256, over TLS pinned to that
// certificate once its fingerprint is checked.

// Install is what the manager hands out to an AP installing from it.
type Install struct {
	Script  []byte        // the installer, with ManagerPlaceholder where the manager's URL goes
	CertPEM []byte        // the manager's TLS certificate, which the AP pins (0033)
	LuCI    bundle.Bundle // LuCI's page for enrolling by hand
	Files   map[string][]byte
}

// ManagerPlaceholder is replaced, in the script it serves, by the URL the
// AP reached the manager at.
const ManagerPlaceholder = "@MANAGER@"

// WithInstall gives the server what it hands out at /install.
func (s *Server) WithInstall(in Install) *Server {
	s.install = &in
	return s
}

// A Host the script may name: a DNS name or an address, and a port. It goes
// into a shell script, so nothing else.
var hostRE = regexp.MustCompile(`^(\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9.-]+)(:[0-9]{1,5})?$`)

// installScript answers with the installer, its manager the one it was
// fetched from.
func (s *Server) installScript(w http.ResponseWriter, r *http.Request) {
	if s.install == nil || s.agents == nil {
		writeError(w, http.StatusNotFound, "this manager hands out no agent (it keeps no agent bundles, or was started without the installer)")
		return
	}
	if !hostRE.MatchString(r.Host) {
		writeError(w, http.StatusBadRequest, "fetch the installer by the manager's name or address")
		return
	}
	script := bytes.ReplaceAll(s.install.Script, []byte(ManagerPlaceholder), []byte("https://"+r.Host))
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(script)
}

// installCert answers with the manager's certificate.
func (s *Server) installCert(w http.ResponseWriter, _ *http.Request) {
	if s.install == nil || len(s.install.CertPEM) == 0 {
		writeError(w, http.StatusNotFound, "no certificate to hand out")
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.install.CertPEM)
}

// installManifest answers with what the installer puts on the AP: the
// manager's release of the agent, and LuCI's page.
func (s *Server) installManifest(w http.ResponseWriter, _ *http.Request) {
	if s.install == nil || s.agents == nil {
		writeError(w, http.StatusNotFound, "this manager hands out no agent")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"version": s.agent.Version,
		"agent":   s.agent,
		"luci":    s.install.LuCI,
	})
}

// installFile answers with one of those files, by SHA-256.
func (s *Server) installFile(w http.ResponseWriter, r *http.Request) {
	sha := r.PathValue("sha256")
	var data []byte
	switch {
	case s.install == nil || s.agents == nil:
	case bundleHas(s.agent, sha):
		d, err := s.agents.File(sha)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			writeError(w, http.StatusInternalServerError, "reading the agent's file")
			return
		}
		data = d
	case bundleHas(s.install.LuCI, sha):
		data = s.install.Files[sha]
	}
	if data == nil {
		writeError(w, http.StatusNotFound, "no such file")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
