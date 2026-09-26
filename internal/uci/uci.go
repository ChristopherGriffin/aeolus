// Package uci reads OpenWrt UCI configuration as `uci export` writes it: the
// form an AP sends for its render check (0039).
//
//	package wireless
//
//	config wifi-iface 'aeolus_sweet_5g'
//		option ssid 'Sweet Spot'
//		list network 'aeolus_sweet'
//
// Words are quoted the way UCI quotes them: single quotes are literal, double
// quotes allow backslash escapes, and adjacent pieces join into one word
// ('it'\”s' is "it's").
package uci

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Config is a set of packages, such as wireless and network.
type Config struct {
	packages map[string]*Package
}

// Package is one config file.
type Package struct {
	Name     string
	Sections []*Section
}

// Section is one "config" block. Anonymous sections have no Name.
type Section struct {
	Type    string
	Name    string
	Line    int
	options map[string]string
	lists   map[string][]string
}

// Package returns a package, or nil.
func (c *Config) Package(name string) *Package { return c.packages[name] }

// Named returns the section with that name, or nil.
func (p *Package) Named(name string) *Section {
	if p == nil {
		return nil
	}
	for _, s := range p.Sections {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// OfType returns the sections of one type, in order.
func (p *Package) OfType(typ string) []*Section {
	if p == nil {
		return nil
	}
	var out []*Section
	for _, s := range p.Sections {
		if s.Type == typ {
			out = append(out, s)
		}
	}
	return out
}

// Option returns a single-valued option.
func (s *Section) Option(name string) (string, bool) {
	v, ok := s.options[name]
	return v, ok
}

// List returns a list option's values. An option that holds one value is a
// list of one.
func (s *Section) List(name string) []string {
	if v, ok := s.options[name]; ok {
		return []string{v}
	}
	return s.lists[name]
}

// Names lists the section's options and lists, sorted.
func (s *Section) Names() []string {
	var out []string
	for n := range s.options {
		out = append(out, n)
	}
	for n := range s.lists {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Flag reads a boolean option the way UCI does: "1", "yes", "on" and "true"
// are true; anything else, or no option, is false.
func (s *Section) Flag(name string) bool {
	switch s.options[name] {
	case "1", "yes", "on", "true", "enabled":
		return true
	}
	return false
}

var (
	nameRE = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	typeRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// Parse reads `uci export` text.
func Parse(text string) (*Config, error) {
	c := &Config{packages: map[string]*Package{}}
	var pkg *Package
	var sec *Section
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		w, err := words(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		if len(w) == 0 {
			continue
		}
		bad := func(format string, args ...any) error {
			return fmt.Errorf("line %d: "+format, append([]any{n}, args...)...)
		}
		switch w[0] {
		case "package":
			if len(w) != 2 || !nameRE.MatchString(w[1]) {
				return nil, bad("package needs one name")
			}
			if c.packages[w[1]] != nil {
				return nil, bad("package %s appears twice", w[1])
			}
			pkg = &Package{Name: w[1]}
			c.packages[w[1]] = pkg
			sec = nil
		case "config":
			if pkg == nil {
				return nil, bad("config before any package")
			}
			if len(w) < 2 || len(w) > 3 || !typeRE.MatchString(w[1]) {
				return nil, bad("config needs a type and at most a name")
			}
			sec = &Section{Type: w[1], Line: n, options: map[string]string{}, lists: map[string][]string{}}
			if len(w) == 3 {
				if !nameRE.MatchString(w[2]) {
					return nil, bad("%q is not a section name", w[2])
				}
				if pkg.Named(w[2]) != nil {
					return nil, bad("section %s appears twice in %s", w[2], pkg.Name)
				}
				sec.Name = w[2]
			}
			pkg.Sections = append(pkg.Sections, sec)
		case "option", "list":
			if sec == nil {
				return nil, bad("%s outside a section", w[0])
			}
			if len(w) != 3 || !nameRE.MatchString(w[1]) {
				return nil, bad("%s needs a name and one value", w[0])
			}
			_, isOption := sec.options[w[1]]
			_, isList := sec.lists[w[1]]
			if w[0] == "option" {
				if isOption || isList {
					return nil, bad("%s is set twice", w[1])
				}
				sec.options[w[1]] = w[2]
			} else {
				if isOption {
					return nil, bad("%s is both an option and a list", w[1])
				}
				sec.lists[w[1]] = append(sec.lists[w[1]], w[2])
			}
		default:
			return nil, bad("unknown keyword %q", w[0])
		}
	}
	return c, nil
}

// words splits a line into words, applying UCI's quoting. A # outside quotes
// at the start of a word begins a comment.
func words(line string) ([]string, error) {
	var out []string
	var cur strings.Builder
	in := false // inside a word
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == ' ' || r == '\t' || r == '\r':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		case r == '#' && !in:
			return out, nil
		case r == '\'':
			in = true
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				cur.WriteRune(rs[j])
				j++
			}
			if j == len(rs) {
				return nil, fmt.Errorf("unterminated single quote")
			}
			i = j
		case r == '"':
			in = true
			j := i + 1
			for ; j < len(rs) && rs[j] != '"'; j++ {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
				}
				cur.WriteRune(rs[j])
			}
			if j == len(rs) {
				return nil, fmt.Errorf("unterminated double quote")
			}
			i = j
		case r == '\\':
			in = true
			if i+1 < len(rs) {
				i++
				cur.WriteRune(rs[i])
			}
		default:
			in = true
			cur.WriteRune(r)
		}
	}
	if in {
		out = append(out, cur.String())
	}
	return out, nil
}

// Redacted is a secret value, as it appears in a redacted copy.
const Redacted = "<secret>"

// Redact returns text with secret values replaced by Redacted: the value of
// every option or list named in names, and any value equal to one of values
// (0041). Every other line is kept exactly. A line that does not parse is
// kept only if it holds none of the values.
func Redact(text string, names map[string]bool, values []string) string {
	secret := map[string]bool{}
	for _, v := range values {
		if v != "" {
			secret[v] = true
		}
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		w, err := words(line)
		if err != nil {
			for v := range secret {
				if strings.Contains(line, v) {
					lines[i] = "# " + Redacted
				}
			}
			continue
		}
		if len(w) == 3 && (w[0] == "option" || w[0] == "list") && (names[w[1]] || secret[w[2]]) {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = indent + w[0] + " " + w[1] + " '" + Redacted + "'"
		}
	}
	return strings.Join(lines, "\n")
}
