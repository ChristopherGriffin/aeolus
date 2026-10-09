package library

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
)

// Template is an AP template (0085): a named set of Locations settings for
// the APs of one or more boards, as OpenWrt names them (arista,c360). It is
// made at a level, the Org or a Locations folder, and offered there and
// below; a folder picks it for a board with templates.<board>.
type Template struct {
	ID     string                             `json:"id"`
	Name   string                             `json:"name"`
	At     hierarchy.NodeID                   `json:"at"`
	Boards []string                           `json:"boards"`
	Values map[hierarchy.Path]hierarchy.Value `json:"values"`
}

var (
	ErrNoTemplate     = errors.New("no such template in the library")
	ErrTemplateExists = errors.New("a template with that ID exists")
	ErrTemplateID     = errors.New("a template's ID is 1 to 64 lowercase letters, digits and hyphens, starting with a letter or digit")
	ErrBoard          = errors.New("a board is named as OpenWrt names it, such as arista,c360: lowercase letters, digits, commas, hyphens, underscores and plus signs, at most 64")
	ErrNoBoards       = errors.New("a template is for at least one board")
	ErrNotSet         = errors.New("the template does not set that field")
)

var (
	templateIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	// No dot: a board is one segment of a field path, templates.<board>.
	boardRE = regexp.MustCompile(`^[a-z0-9][a-z0-9,_+-]{0,63}$`)
)

// TemplateIDOK says whether id may name a template.
func TemplateIDOK(id string) bool { return templateIDRE.MatchString(id) }

// BoardOK says whether b may name a board.
func BoardOK(b string) bool { return boardRE.MatchString(b) }

// ForBoard says whether the template is for board b.
func (t Template) ForBoard(b string) bool { return slices.Contains(t.Boards, b) }

func copyTemplate(t *Template) *Template {
	cp := *t
	cp.Boards = append([]string(nil), t.Boards...)
	cp.Values = make(map[hierarchy.Path]hierarchy.Value, len(t.Values))
	for p, v := range t.Values {
		cp.Values[p] = v // values are never modified in place
	}
	return &cp
}

func checkBoards(boards []string) error {
	if len(boards) == 0 {
		return ErrNoBoards
	}
	for _, b := range boards {
		if !BoardOK(b) {
			return fmt.Errorf("%w: %q", ErrBoard, b)
		}
	}
	return nil
}

// Template returns a copy of a template.
func (l *Library) Template(id string) (Template, bool) {
	t, ok := l.templates[id]
	if !ok {
		return Template{}, false
	}
	return *copyTemplate(t), true
}

// Templates returns copies of every template, sorted by ID.
func (l *Library) Templates() []Template {
	out := make([]Template, 0, len(l.templates))
	for _, t := range l.templates {
		out = append(out, *copyTemplate(t))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AddTemplate adds a template with no values. The caller checks its level.
func (l *Library) AddTemplate(t Template) error {
	if !TemplateIDOK(t.ID) {
		return ErrTemplateID
	}
	if _, ok := l.templates[t.ID]; ok {
		return fmt.Errorf("%w: %s", ErrTemplateExists, t.ID)
	}
	if err := checkBoards(t.Boards); err != nil {
		return err
	}
	next := copyTemplate(&t)
	next.Values = map[hierarchy.Path]hierarchy.Value{}
	l.templates[t.ID] = next
	return nil
}

// EditTemplate renames a template and sets its boards. It returns what it
// was.
func (l *Library) EditTemplate(id, name string, boards []string) (Template, error) {
	t, ok := l.templates[id]
	if !ok {
		return Template{}, fmt.Errorf("%w: %s", ErrNoTemplate, id)
	}
	if err := checkBoards(boards); err != nil {
		return Template{}, err
	}
	prev := *copyTemplate(t)
	t.Name, t.Boards = name, append([]string(nil), boards...)
	return prev, nil
}

// SetTemplateValue sets one of a template's fields. It returns the value it
// replaced, if any.
func (l *Library) SetTemplateValue(id string, p hierarchy.Path, v hierarchy.Value) (hierarchy.Value, bool, error) {
	t, ok := l.templates[id]
	if !ok {
		return nil, false, fmt.Errorf("%w: %s", ErrNoTemplate, id)
	}
	prev, had := t.Values[p]
	t.Values[p] = v
	return prev, had, nil
}

// UnsetTemplateValue deletes one of a template's fields, and returns it.
func (l *Library) UnsetTemplateValue(id string, p hierarchy.Path) (hierarchy.Value, error) {
	t, ok := l.templates[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoTemplate, id)
	}
	prev, had := t.Values[p]
	if !had {
		return nil, fmt.Errorf("%w: %s", ErrNotSet, p)
	}
	delete(t.Values, p)
	return prev, nil
}

// RemoveTemplate deletes a template. The caller checks nothing picks it.
func (l *Library) RemoveTemplate(id string) (Template, error) {
	t, ok := l.templates[id]
	if !ok {
		return Template{}, fmt.Errorf("%w: %s", ErrNoTemplate, id)
	}
	delete(l.templates, id)
	return *copyTemplate(t), nil
}
