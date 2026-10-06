// Package bundle is the agent bundles the manager offers its APs (0079): a
// manifest of the agent's files, each with its place on the AP, its mode and
// its SHA-256, and the release it came with. A manager build carries its
// own; the store keeps the last few releases', so a folder pinned to an older
// one keeps getting it after the manager moves on.
package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// File is one of a bundle's files.
type File struct {
	Path   string `json:"path"` // its place on the AP, such as /usr/sbin/aeolus-agent
	Mode   string `json:"mode"` // 0755 or 0644
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Bundle is one release's agent: its files, and the hash that names it.
type Bundle struct {
	Version string    `json:"version"`
	Hash    string    `json:"hash"`
	Stored  time.Time `json:"stored"`
	Files   []File    `json:"files"`
}

// Paths an agent file may have on an AP: the agent's own places, and nothing
// else, so a manifest can't name /etc/shadow.
var pathRE = regexp.MustCompile(`^/(usr/sbin|usr/libexec|usr/share/ucode/aeolus|etc/init\.d|etc/hotplug\.d/[a-z0-9-]+|lib/upgrade/keep\.d)/[A-Za-z0-9._-]+$`)

// PathOK says whether p may be an agent file's place on an AP.
func PathOK(p string) bool {
	return pathRE.MatchString(p) && !strings.Contains(p, "..")
}

// mode is a file's mode on the AP: the programs and the init script run.
func mode(p string) string {
	for _, dir := range []string{"/usr/sbin/", "/usr/libexec/", "/etc/init.d/"} {
		if strings.HasPrefix(p, dir) {
			return "0755"
		}
	}
	return "0644"
}

// hash names a bundle by its files: the SHA-256 of a line for each, its
// path, mode and SHA-256, in path order. The release isn't in it, so two
// releases with the same agent are the same bundle.
func hash(files []File) string {
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s %s %s\n", f.Path, f.Mode, f.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// FromFS makes the bundle of the files under root in fsys, each at its path
// below root on the AP, for the given release. It returns the files'
// contents, by their SHA-256.
func FromFS(fsys fs.FS, root, version string) (Bundle, map[string][]byte, error) {
	b := Bundle{Version: version}
	contents := map[string][]byte{}
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		place := "/" + strings.TrimPrefix(p, root+"/")
		if !PathOK(place) {
			return fmt.Errorf("agent file %s: not a place an agent file may have", place)
		}
		sum := sha256.Sum256(data)
		f := File{Path: place, Mode: mode(place), SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))}
		b.Files = append(b.Files, f)
		contents[f.SHA256] = data
		return nil
	})
	if err != nil {
		return Bundle{}, nil, err
	}
	if len(b.Files) == 0 {
		return Bundle{}, nil, errors.New("no agent files")
	}
	sort.Slice(b.Files, func(i, j int) bool { return b.Files[i].Path < b.Files[j].Path })
	b.Hash = hash(b.Files)
	return b, contents, nil
}

// Store keeps bundles in a directory: each manifest by its release, under
// manifests/, and the files by their SHA-256, under files/.
type Store struct {
	dir  string
	keep int
	mu   sync.Mutex
}

// Open opens the store in dir, making it if need be, keeping the newest
// keep releases.
func Open(dir string, keep int) (*Store, error) {
	for _, d := range []string{"manifests", "files"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return nil, err
		}
	}
	return &Store{dir: dir, keep: keep}, nil
}

var versionRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func (s *Store) manifest(version string) string {
	return filepath.Join(s.dir, "manifests", version+".json")
}

// Put keeps a release's bundle, with its files' contents. A release kept
// before with the same files keeps the time it was first kept, so the
// newest are those the manager ran most recently.
func (s *Store) Put(b Bundle, contents map[string][]byte, now time.Time) (Bundle, error) {
	if !versionRE.MatchString(b.Version) {
		return Bundle{}, fmt.Errorf("bundle release %q: not a release name", b.Version)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, err := s.read(b.Version); err == nil && old.Hash == b.Hash {
		b.Stored = old.Stored
	} else {
		b.Stored = now.UTC()
	}
	for _, f := range b.Files {
		data, ok := contents[f.SHA256]
		if !ok {
			return Bundle{}, fmt.Errorf("bundle %s: no contents for %s", b.Version, f.Path)
		}
		if err := writeAtomic(filepath.Join(s.dir, "files", f.SHA256), data); err != nil {
			return Bundle{}, err
		}
	}
	raw, err := json.MarshalIndent(b, "", "\t")
	if err != nil {
		return Bundle{}, err
	}
	if err := writeAtomic(s.manifest(b.Version), raw); err != nil {
		return Bundle{}, err
	}
	return b, s.prune()
}

func writeAtomic(p string, data []byte) error {
	tmp := p + ".new"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (s *Store) read(version string) (Bundle, error) {
	raw, err := os.ReadFile(s.manifest(version))
	if err != nil {
		return Bundle{}, err
	}
	var b Bundle
	err = json.Unmarshal(raw, &b)
	return b, err
}

// list is every kept bundle, newest first.
func (s *Store) list() ([]Bundle, error) {
	names, err := filepath.Glob(filepath.Join(s.dir, "manifests", "*.json"))
	if err != nil {
		return nil, err
	}
	var out []Bundle
	for _, n := range names {
		b, err := s.read(strings.TrimSuffix(filepath.Base(n), ".json"))
		if err != nil {
			continue // a manifest it can't read is no release to offer
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Stored.After(out[j].Stored) })
	return out, nil
}

// prune drops all but the newest releases, and the files no kept release
// has.
func (s *Store) prune() error {
	all, err := s.list()
	if err != nil {
		return err
	}
	used := map[string]bool{}
	for i, b := range all {
		if i >= s.keep {
			if err := os.Remove(s.manifest(b.Version)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			continue
		}
		for _, f := range b.Files {
			used[f.SHA256] = true
		}
	}
	files, err := os.ReadDir(filepath.Join(s.dir, "files"))
	if err != nil {
		return err
	}
	for _, f := range files {
		if !used[f.Name()] {
			os.Remove(filepath.Join(s.dir, "files", f.Name()))
		}
	}
	return nil
}

// Versions is every kept bundle, newest first.
func (s *Store) Versions() ([]Bundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list()
}

// Find is the kept bundle a pin names: a release exactly, or its tag, so
// v0.49.0 finds v0.49.0-1a2b3c4; the newest such.
func (s *Store) Find(pin string) (Bundle, bool) {
	all, err := s.Versions()
	if err != nil {
		return Bundle{}, false
	}
	for _, b := range all {
		if b.Version == pin || strings.HasPrefix(b.Version, pin+"-") {
			return b, true
		}
	}
	return Bundle{}, false
}

var shaRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// File is a kept file's contents, by its SHA-256.
func (s *Store) File(sha string) ([]byte, error) {
	if !shaRE.MatchString(sha) {
		return nil, fs.ErrNotExist
	}
	return os.ReadFile(filepath.Join(s.dir, "files", path.Base(sha)))
}
