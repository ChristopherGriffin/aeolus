package change

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/library"
)

// AP templates (0085). A template is made at a level, the Org or a Locations
// folder, and offered there and below; a Locations field, templates.<board>,
// picks one for a board's APs, and inherits like any other. An AP takes the
// template picked nearest above it for its board. The template's values
// count as set at the node that picks it, ahead of that node's own: what is
// set below it, or locked, replaces them.

// TemplatesPrefix starts the Locations fields that pick templates.
const TemplatesPrefix = "templates."

// NotifyPrefix starts the Locations fields that say where the manager sends
// alerts (0101): the manager's, as the template picks are.
const NotifyPrefix = "notify."

// RoguesPrefix starts the Locations fields that say what the manager makes
// of networks the APs hear (0106): the manager's too.
const RoguesPrefix = "rogues."

// TemplatePath is the Locations field that picks a board's template.
func TemplatePath(board string) hierarchy.Path { return hierarchy.Path(TemplatesPrefix + board) }

// templateFields are what a template may set: Locations settings, but not
// tunnels (0055), names, the service folders, templates, or the agent's
// release (0079).
var templateFields = []string{"radio.", "ports.", "uplink.", "system.", "rrm.", "apc."}

// TemplateFieldOK says whether a template may set p.
func TemplateFieldOK(p hierarchy.Path) bool {
	s := string(p)
	if s == "system.agent" || strings.HasPrefix(s, "system.agent.") {
		return false
	}
	for _, pre := range templateFields {
		if strings.HasPrefix(s, pre) {
			return true
		}
	}
	return false
}

var (
	ErrNoTemplateID    = errors.New("change needs a template")
	ErrTemplateLevel   = errors.New("a template is made at the Org or a Locations folder, outside Landing Zone")
	ErrTemplateField   = errors.New("a template sets radios, ports, the uplink, system settings, radio resource management and power control, not tunnels, names, service folders, templates or the agent's release")
	ErrTemplateName    = errors.New("a template's name is 1 to 64 characters, none of them control characters")
	ErrDefaultTemplate = errors.New("the manager makes a template only for a kind of AP no template is for, at the Org, picked there")
)

// TemplatePickError says why a node's templates.<board> picks no template it
// may.
type TemplatePickError struct {
	Node     hierarchy.NodeID
	Board    string
	Template any
	Why      string
}

func (e *TemplatePickError) Error() string {
	return fmt.Sprintf("%s at %s picks %v: %s", TemplatePath(e.Board), e.Node, e.Template, e.Why)
}

func validateTemplate(op Op) error {
	if op.Template == "" {
		return ErrNoTemplateID
	}
	switch op.Kind {
	case AddTemplate:
		if op.Parent == "" {
			return ErrNoNode
		}
		// Made with its settings, as an imported template is (0090): only
		// values, never path and value.
		if op.Path != "" || len(op.Value) > 0 {
			return ErrTwoForms
		}
		for p := range op.Values {
			if p == "" {
				return ErrNoPath
			}
		}
		fallthrough
	case EditTemplate:
		if err := folderName(op.Name); err != nil {
			return ErrTemplateName
		}
		if len(op.Boards) == 0 {
			return library.ErrNoBoards
		}
	case SetTemplate:
		if len(op.Values) > 0 && (op.Path != "" || len(op.Value) > 0) {
			return ErrTwoForms
		}
		for _, f := range op.Fields() {
			if f.Path == "" {
				return ErrNoPath
			}
		}
	case UnsetTemplate:
		if len(op.Paths) > 0 && op.Path != "" {
			return ErrTwoForms
		}
		for _, p := range op.Unsets() {
			if p == "" {
				return ErrNoPath
			}
		}
	}
	return nil
}

// templateView is how a template's definition is shown in a change's effect.
func templateView(t library.Template) map[string]any {
	return map[string]any{"id": t.ID, "name": t.Name, "at": string(t.At), "boards": t.Boards}
}

func applyTemplate(s *State, op Op) (Effect, error) {
	lib, t := s.Library, s.Org.Locations
	switch op.Kind {
	case AddTemplate:
		n, ok := t.Node(op.Parent)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Parent)
		}
		if n.Kind == hierarchy.KindAP || t.InIsolated(op.Parent) {
			return Effect{}, ErrTemplateLevel
		}
		tm := library.Template{ID: op.Template, Name: op.Name, At: op.Parent, Boards: op.Boards}
		if err := lib.AddTemplate(tm); err != nil {
			return Effect{}, err
		}
		after := templateView(tm)
		// Its settings, made with it: an imported template (0090).
		if len(op.Values) > 0 {
			values := map[string]any{}
			for _, f := range op.Fields() {
				if !TemplateFieldOK(f.Path) {
					return Effect{}, fmt.Errorf("%w: %s", ErrTemplateField, f.Path)
				}
				v, err := decode(f.Value)
				if err != nil {
					return Effect{}, err
				}
				if _, _, err := lib.SetTemplateValue(op.Template, f.Path, v); err != nil {
					return Effect{}, err
				}
				values[string(f.Path)] = v
			}
			after["values"] = values
		}
		// Default: picked where it is made, for each of its boards that
		// nothing is picked for there yet.
		if op.Default {
			var picked []string
			for _, b := range op.Boards {
				if _, set := t.Own(op.Parent, TemplatePath(b)); set {
					continue
				}
				if err := t.Set(op.Parent, TemplatePath(b), op.Template); err != nil {
					return Effect{}, err
				}
				picked = append(picked, b)
			}
			after["picked_for"] = picked
		}
		return Effect{After: after}, nil
	case EditTemplate:
		prev, err := lib.EditTemplate(op.Template, op.Name, op.Boards)
		if err != nil {
			return Effect{}, err
		}
		next, _ := lib.Template(op.Template)
		return Effect{Before: templateView(prev), After: templateView(next)}, nil
	case SetTemplate:
		if _, ok := lib.Template(op.Template); !ok {
			return Effect{}, fmt.Errorf("%w: %s", library.ErrNoTemplate, op.Template)
		}
		before, after := map[string]any{}, map[string]any{}
		for _, f := range op.Fields() {
			if !TemplateFieldOK(f.Path) {
				return Effect{}, fmt.Errorf("%w: %s", ErrTemplateField, f.Path)
			}
			v, err := decode(f.Value)
			if err != nil {
				return Effect{}, err
			}
			prev, had, err := lib.SetTemplateValue(op.Template, f.Path, v)
			if err != nil {
				return Effect{}, err
			}
			if had {
				before[string(f.Path)] = prev
			}
			after[string(f.Path)] = v
		}
		if len(before) == 0 {
			return Effect{After: after}, nil
		}
		return Effect{Before: before, After: after}, nil
	case UnsetTemplate:
		tm, ok := lib.Template(op.Template)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", library.ErrNoTemplate, op.Template)
		}
		before := map[string]any{}
		for _, p := range op.Unsets() {
			v, had := tm.Values[p]
			if !had {
				return Effect{}, fmt.Errorf("%w: %s", library.ErrNotSet, p)
			}
			before[string(p)] = v
		}
		for _, p := range op.Unsets() {
			lib.UnsetTemplateValue(op.Template, p) // each is set, so none fails
		}
		return Effect{Before: before}, nil
	case RemoveTemplate:
		if by := templatePickers(s, op.Template); len(by) > 0 {
			return Effect{}, &InUseError{What: "template " + op.Template, By: by}
		}
		prev, err := lib.RemoveTemplate(op.Template)
		if err != nil {
			return Effect{}, err
		}
		return Effect{Before: templateView(prev)}, nil
	}
	return Effect{}, fmt.Errorf("%w: %q", ErrUnknownKind, op.Kind)
}

// templatePickers lists the nodes whose templates.<board> picks a template.
func templatePickers(s *State, id string) []string {
	var by []string
	s.Org.Locations.EachSet(func(n hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
		if strings.HasPrefix(string(p), TemplatesPrefix) && v == id {
			by = append(by, string(n)+" "+string(p))
		}
	})
	sort.Strings(by)
	return by
}

// checkTemplates checks every templates.<board> set in Locations: it picks
// a template for that board, offered where it is set, that is, made there
// or above.
func checkTemplates(s *State) error {
	t := s.Org.Locations
	var errs []error
	t.EachSet(func(n hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
		board, ok := strings.CutPrefix(string(p), TemplatesPrefix)
		if !ok {
			return
		}
		id, _ := v.(string)
		tm, ok := s.Library.Template(id)
		switch {
		case !ok:
			errs = append(errs, &TemplatePickError{Node: n, Board: board, Template: v, Why: "no such template"})
		case !tm.ForBoard(board):
			errs = append(errs, &TemplatePickError{Node: n, Board: board, Template: id, Why: "it is not for that board"})
		case !slices.Contains(t.Ancestry(n), tm.At):
			errs = append(errs, &TemplatePickError{Node: n, Board: board, Template: id, Why: fmt.Sprintf("it is made at %s, which is not this node or above it", tm.At)})
		}
	})
	if len(errs) == 0 {
		return nil
	}
	sort.Slice(errs, func(i, j int) bool { return errs[i].Error() < errs[j].Error() })
	return errs[0]
}

// Model is the model an AP said it is when it enrolled (0033), such as
// "Arista C-360", or "".
func (s *State) Model(ap hierarchy.NodeID) string {
	var f struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(s.Facts[ap], &f) != nil {
		return ""
	}
	return f.Model
}

// Board is the board an AP said it is when it enrolled (0033), or "".
func (s *State) Board(ap hierarchy.NodeID) string {
	var f struct {
		Board string `json:"board"`
	}
	if json.Unmarshal(s.Facts[ap], &f) != nil {
		return ""
	}
	return f.Board
}

// TemplateFor returns the template an AP takes (0085), and the node that
// picks it: the nearest templates.<board> above it, for its board.
func (s *State) TemplateFor(ap hierarchy.NodeID) (library.Template, hierarchy.NodeID, bool) {
	board := s.Board(ap)
	if !library.BoardOK(board) {
		return library.Template{}, "", false
	}
	r, ok := s.Org.Locations.Resolve(ap, TemplatePath(board))
	if !ok {
		return library.Template{}, "", false
	}
	id, _ := r.Value.(string)
	tm, ok := s.Library.Template(id)
	return tm, r.From, ok
}

// ResolveAP resolves an AP as the Org does, with its template (0085). A
// template's value takes a field unless something set below the node that
// picks the template, or locked, sets it; each field it loses is listed in
// the config's Template. The fields that pick templates are left out: they
// are the manager's, not the AP's.
func (s *State) ResolveAP(ap hierarchy.NodeID) (hierarchy.APConfig, error) {
	cfg, err := s.Org.ResolveAP(ap)
	if err != nil || cfg.Unassigned {
		return cfg, err
	}
	for p := range cfg.Location {
		if strings.HasPrefix(string(p), TemplatesPrefix) || strings.HasPrefix(string(p), NotifyPrefix) || strings.HasPrefix(string(p), RoguesPrefix) {
			delete(cfg.Location, p)
		}
	}
	chain := s.Org.Locations.Chain(ap)
	tm, at, ok := s.TemplateFor(ap)
	if !ok {
		foldBoards(&cfg, s.Board(ap), chain)
		return cfg, nil
	}
	pos := slices.Index(chain, at)
	use := &hierarchy.TemplateUse{ID: tm.ID, Name: tm.Name, At: at, Replaced: []hierarchy.Override{}}
	paths := make([]hierarchy.Path, 0, len(tm.Values))
	for p := range tm.Values {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i] < paths[j] })
	for _, p := range paths {
		if r, set := cfg.Location[p]; set {
			if i := slices.Index(chain, r.From); r.Origin == hierarchy.OriginLocked || i >= 0 && i < pos {
				use.Replaced = append(use.Replaced, hierarchy.Override{Node: r.From, Path: p, Value: r.Value})
				continue
			}
		}
		cfg.Location[p] = hierarchy.Resolved{Value: tm.Values[p], From: at, Origin: hierarchy.OriginTemplate}
	}
	cfg.Template = use
	// A kind of AP's own settings, set on the folders (0092), go after its
	// template, which they replace where set on the node that picks it or
	// below.
	foldBoards(&cfg, s.Board(ap), chain)
	return cfg, nil
}

// TemplateIDFor makes a template's ID from a board's name: arista,c360 is
// arista-c360. It is a new ID: one taken gets a number.
func TemplateIDFor(lib *library.Library, board string) string {
	base := strings.Trim(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, board), "-")
	if len(base) > 60 {
		base = base[:60]
	}
	if base == "" {
		base = "ap"
	}
	id := base
	for i := 2; ; i++ {
		if _, taken := lib.Template(id); !taken {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

// NewKinds are the changes the manager makes itself for the kinds of AP
// adopted that no template is for yet (0085): an empty template each, at the
// Org, named for the AP's model, and picked there for its board. A change
// only says what to make; the manager commits them, in its own name.
func NewKinds(s *State) []Op {
	if s == nil {
		return nil
	}
	t := s.Org.Locations
	root := t.Root()
	has := map[string]bool{}
	for _, tm := range s.Library.Templates() {
		for _, b := range tm.Boards {
			has[b] = true
		}
	}
	var ops []Op
	for _, ap := range t.APs() {
		if t.InIsolated(ap) {
			continue // not adopted
		}
		board := s.Board(ap)
		if !library.BoardOK(board) || has[board] {
			continue
		}
		has[board] = true
		ops = append(ops, Op{Kind: AddTemplate, Template: TemplateIDFor(s.Library, board), Name: modelName(s, ap, board), Parent: root, Boards: []string{board}, Default: true})
	}
	return ops
}

// modelName is a new template's name: the model the AP reported, else its
// board.
func modelName(s *State, ap hierarchy.NodeID, board string) string {
	var f struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(s.Facts[ap], &f) == nil && folderName(f.Model) == nil {
		return f.Model
	}
	return board
}
