// Package schema checks fields and whole AP configs against the v1 field list
// (0027), and keeps secrets out of the change log in plain text.
//
// Two levels of checking:
//   - Field: every set is checked on its own before it is committed. A path
//     must name a leaf of the schema in the tree that owns it (0013), and the
//     value must be valid for that leaf.
//   - Whole config: an AP's assembled config is checked against the full
//     schema, including rules that tie fields together (a WPA2 network needs a
//     passphrase, a VXLAN transport needs a concentrator and a VNI). An
//     incomplete config is a check result for that AP, not a reason to refuse
//     the change that led to it (0029).
package schema

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/secret"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed v1.json
var v1JSON []byte

var (
	ErrUnknownField = errors.New("unknown field")
	ErrNotAField    = errors.New("not a field; set the fields inside it")
	ErrReadOnly     = errors.New("filled in by the manager; it cannot be set")
	ErrWrongTree    = errors.New("field belongs to the other tree")
	ErrPlainSecret  = errors.New("secret fields must be sealed before they are committed")
	ErrNoKey        = errors.New("no secret key to seal this field with")
)

// FieldError is a problem with a field path or its value.
type FieldError struct {
	Path hierarchy.Path
	Err  error
}

func (e *FieldError) Error() string { return fmt.Sprintf("%s: %v", e.Path, e.Err) }
func (e *FieldError) Unwrap() error { return e.Err }

// Field is one settable field of the schema.
type Field struct {
	Path    hierarchy.Path
	Tree    change.TreeName
	Secret  bool
	pointer string
}

// Schema is a compiled field list.
type Schema struct {
	id       string
	raw      map[string]any
	compiler *jsonschema.Compiler
	whole    *jsonschema.Schema

	mu     sync.Mutex
	leaves map[string]*jsonschema.Schema
}

var (
	loadV1 sync.Once
	v1Sch  *Schema
	v1Err  error
)

// V1 returns the v1 field list.
func V1() (*Schema, error) {
	loadV1.Do(func() { v1Sch, v1Err = Load(v1JSON) })
	return v1Sch, v1Err
}

// Load compiles a schema document.
func Load(data []byte) (*Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	raw, ok := doc.(map[string]any)
	if !ok {
		return nil, errors.New("schema is not a JSON object")
	}
	id, _ := raw["$id"].(string)
	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, doc); err != nil {
		return nil, err
	}
	whole, err := c.Compile(id)
	if err != nil {
		return nil, err
	}
	return &Schema{id: id, raw: raw, compiler: c, whole: whole, leaves: map[string]*jsonschema.Schema{}}, nil
}

// Field resolves a path such as "network.sweet.transport.primary.vni" to the
// schema leaf it names.
func (s *Schema) Field(p hierarchy.Path) (Field, error) {
	segs := strings.Split(string(p), ".")
	node, ptr := s.deref(s.raw, "")
	f := Field{Path: p}
	for i, seg := range segs {
		if seg == "" {
			return Field{}, &FieldError{Path: p, Err: ErrUnknownField}
		}
		if node["type"] != "object" {
			return Field{}, &FieldError{Path: p, Err: fmt.Errorf("%w: %s is a field, not a group", ErrUnknownField, strings.Join(segs[:i], "."))}
		}
		if props, _ := node["properties"].(map[string]any); props[seg] != nil {
			node, _ = props[seg].(map[string]any)
			ptr += "/properties/" + escape(seg)
		} else if extra, ok := node["additionalProperties"].(map[string]any); ok {
			if !s.nameAllowed(node, seg) {
				return Field{}, &FieldError{Path: p, Err: fmt.Errorf("%w: %q is not a valid name here", ErrUnknownField, seg)}
			}
			node = extra
			ptr += "/additionalProperties"
		} else {
			return Field{}, &FieldError{Path: p, Err: fmt.Errorf("%w: %s", ErrUnknownField, strings.Join(segs[:i+1], "."))}
		}
		if i == 0 {
			if node["readOnly"] == true {
				return Field{}, &FieldError{Path: p, Err: ErrReadOnly}
			}
			tree, _ := node["x-aeolus-tree"].(string)
			f.Tree = change.TreeName(tree)
		}
		node, ptr = s.deref(node, ptr)
	}
	if node["type"] == "object" {
		return Field{}, &FieldError{Path: p, Err: ErrNotAField}
	}
	f.Secret = node["writeOnly"] == true
	f.pointer = ptr
	return f, nil
}

// Check validates a plain value for a field.
func (s *Schema) Check(p hierarchy.Path, v any) error {
	f, err := s.Field(p)
	if err != nil {
		return err
	}
	return s.checkLeaf(f, v)
}

func (s *Schema) checkLeaf(f Field, v any) error {
	sch, err := s.leaf(f.pointer)
	if err != nil {
		return err
	}
	if err := sch.Validate(v); err != nil {
		return &FieldError{Path: f.Path, Err: tidy(err)}
	}
	return nil
}

// Prepare turns a value submitted for a field into the value to commit: it is
// validated, and sealed with box if the field is secret. Callers commit only
// what Prepare returns.
func (s *Schema) Prepare(p hierarchy.Path, raw json.RawMessage, box *secret.Box) (json.RawMessage, error) {
	f, err := s.Field(p)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, &FieldError{Path: p, Err: err}
	}
	if err := s.checkLeaf(f, v); err != nil {
		return nil, err
	}
	if f.Secret {
		if box == nil {
			return nil, &FieldError{Path: p, Err: ErrNoKey}
		}
		sealed, err := box.Seal(string(p), v)
		if err != nil {
			return nil, err
		}
		return json.Marshal(sealed)
	}
	return json.Marshal(v)
}

// CheckOp is the change log's commit guard: every field a change touches must
// exist in the tree it targets, set values must be valid, and secret values
// must arrive sealed. It is not applied on replay.
func (s *Schema) CheckOp(op change.Op) error {
	if op.Kind == change.SetConcentrator {
		return s.checkConcentrator(op.Value)
	}
	switch op.Kind {
	case change.Set:
		for _, f := range op.Fields() {
			if err := s.checkField(op.Tree, f.Path, f.Value, true); err != nil {
				return err
			}
		}
	case change.Unset:
		for _, p := range op.Unsets() {
			if err := s.checkField(op.Tree, p, nil, false); err != nil {
				return err
			}
		}
	case change.Lock, change.Unlock:
		return s.checkField(op.Tree, op.Path, nil, false)
	}
	return nil
}

// checkField checks one field a change touches, and for a set, its value.
func (s *Schema) checkField(tree change.TreeName, p hierarchy.Path, raw json.RawMessage, set bool) error {
	if tree == change.Locations && p == hierarchy.ServicesPath {
		return nil // assign-services is validated by the engine; unset/lock/unlock are fine
	}
	f, err := s.Field(p)
	if err != nil {
		return err
	}
	if f.Tree != tree {
		return &FieldError{Path: p, Err: fmt.Errorf("%w: it is set in %s", ErrWrongTree, f.Tree)}
	}
	if !set {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return &FieldError{Path: p, Err: err}
	}
	if f.Secret {
		if !secret.IsSealed(v) {
			return &FieldError{Path: p, Err: ErrPlainSecret}
		}
		return nil
	}
	return s.checkLeaf(f, v)
}

// checkConcentrator validates a library concentrator's definition (0023).
func (s *Schema) checkConcentrator(raw json.RawMessage) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return &FieldError{Path: "concentrator", Err: err}
	}
	sch, err := s.leaf("/$defs/libraryConcentrator")
	if err != nil {
		return err
	}
	if err := sch.Validate(v); err != nil {
		return &FieldError{Path: "concentrator", Err: tidy(err)}
	}
	return nil
}

// CheckDocument validates an AP's whole assembled config.
func (s *Schema) CheckDocument(doc map[string]any) error {
	if err := s.whole.Validate(doc); err != nil {
		return tidy(err)
	}
	return nil
}

// Problems lists what is wrong with a whole or partial config, one entry per
// problem, or nil when it passes. The API reports these per AP and per node
// so the UI can keep an admin on a page until they are fixed (0029).
func (s *Schema) Problems(doc map[string]any) []string {
	err := s.whole.Validate(doc)
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if errors.As(err, &ve) {
		return problems(ve)
	}
	return []string{err.Error()}
}

// Assemble nests flat field values ("radio.5g.width") into a config document.
// reveal opens sealed secrets; with a nil reveal they stay sealed.
func Assemble(fields map[string]any, reveal func(path string, v any) (any, error)) (map[string]any, error) {
	doc := map[string]any{}
	for path, v := range fields {
		if reveal != nil && secret.IsSealed(v) {
			var err error
			if v, err = reveal(path, v); err != nil {
				return nil, err
			}
		}
		if err := insert(doc, strings.Split(path, "."), v); err != nil {
			return nil, err
		}
	}
	return doc, nil
}

func insert(m map[string]any, segs []string, v any) error {
	for i, seg := range segs[:len(segs)-1] {
		next, exists := m[seg]
		if !exists {
			child := map[string]any{}
			m[seg] = child
			m = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("%s holds a value and cannot also hold fields", strings.Join(segs[:i+1], "."))
		}
		m = child
	}
	last := segs[len(segs)-1]
	if _, exists := m[last]; exists {
		return fmt.Errorf("%s is set twice", strings.Join(segs, "."))
	}
	m[last] = v
	return nil
}

// Leaves lists every field of an AP's config, sorted, with "*" where a name
// is chosen (a network or port ID): "network.*.ssid". Parts filled in by the
// manager are included.
func (s *Schema) Leaves() []string {
	var out []string
	var walk func(node map[string]any, prefix string)
	walk = func(node map[string]any, prefix string) {
		node, _ = s.deref(node, "")
		if node["type"] != "object" {
			out = append(out, prefix)
			return
		}
		join := func(seg string) string {
			if prefix == "" {
				return seg
			}
			return prefix + "." + seg
		}
		props, _ := node["properties"].(map[string]any)
		for name, child := range props {
			if m, ok := child.(map[string]any); ok {
				walk(m, join(name))
			}
		}
		if extra, ok := node["additionalProperties"].(map[string]any); ok {
			walk(extra, join("*"))
		}
	}
	walk(s.raw, "")
	sort.Strings(out)
	return out
}

// Description is what a client needs to offer the schema's fields for
// editing (0048): each settable field as its JSON Schema, with the tree it
// is set in, and the pattern a name must match where a path takes any name.
type Description struct {
	Fields map[string]map[string]any `json:"fields"` // by path, "*" for any name; "x-aeolus-tree" says which tree
	Names  map[string]string         `json:"names"`  // by the path before the "*"
}

// Describe lists every field that can be set. Fields the manager fills in
// itself are left out.
func (s *Schema) Describe() Description {
	d := Description{Fields: map[string]map[string]any{}, Names: map[string]string{}}
	var walk func(node map[string]any, prefix, tree string)
	walk = func(node map[string]any, prefix, tree string) {
		node, _ = s.deref(node, "")
		if node["type"] != "object" {
			f := make(map[string]any, len(node)+1)
			for k, v := range node {
				f[k] = v
			}
			f["x-aeolus-tree"] = tree
			d.Fields[prefix] = f
			return
		}
		join := func(seg string) string {
			if prefix == "" {
				return seg
			}
			return prefix + "." + seg
		}
		props, _ := node["properties"].(map[string]any)
		for name, child := range props {
			m, ok := child.(map[string]any)
			if !ok {
				continue
			}
			t := tree
			if prefix == "" {
				if m["readOnly"] == true {
					continue
				}
				t, _ = m["x-aeolus-tree"].(string)
			}
			walk(m, join(name), t)
		}
		if extra, ok := node["additionalProperties"].(map[string]any); ok {
			if pn, ok := node["propertyNames"].(map[string]any); ok {
				if pn, _ = s.deref(pn, ""); pn["pattern"] != nil {
					d.Names[prefix], _ = pn["pattern"].(string)
				}
			}
			walk(extra, join("*"), tree)
		}
	}
	walk(s.raw, "", "")
	return d
}

// deref follows local $refs, returning the node and its JSON pointer.
func (s *Schema) deref(node map[string]any, ptr string) (map[string]any, string) {
	for {
		ref, ok := node["$ref"].(string)
		if !ok || !strings.HasPrefix(ref, "#/") {
			return node, ptr
		}
		ptr = ref[1:]
		node = s.at(ptr)
	}
}

func (s *Schema) at(ptr string) map[string]any {
	var cur any = s.raw
	for _, seg := range strings.Split(strings.TrimPrefix(ptr, "/"), "/") {
		m, _ := cur.(map[string]any)
		cur = m[unescape(seg)]
	}
	m, _ := cur.(map[string]any)
	return m
}

func (s *Schema) nameAllowed(node map[string]any, name string) bool {
	pn, ok := node["propertyNames"].(map[string]any)
	if !ok {
		return true
	}
	pn, _ = s.deref(pn, "")
	pattern, ok := pn["pattern"].(string)
	if !ok {
		return true
	}
	return regexp.MustCompile(pattern).MatchString(name)
}

func (s *Schema) leaf(ptr string) (*jsonschema.Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sch, ok := s.leaves[ptr]; ok {
		return sch, nil
	}
	sch, err := s.compiler.Compile(s.id + "#" + ptr)
	if err != nil {
		return nil, err
	}
	s.leaves[ptr] = sch
	return sch, nil
}

func escape(seg string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(seg)
}

func unescape(seg string) string {
	return strings.NewReplacer("~1", "/", "~0", "~").Replace(seg)
}

var english = message.NewPrinter(language.English)

// tidy turns a validation error into what an admin needs: each problem, where
// it is (in dotted field names), and nothing about the validator's internals.
// Alternatives from oneOf/anyOf are joined with "or".
func tidy(err error) error {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	return errors.New(strings.Join(problems(ve), "; "))
}

func problems(e *jsonschema.ValidationError) []string {
	if len(e.Causes) == 0 {
		msg := e.ErrorKind.LocalizedString(english)
		if loc := strings.Join(e.InstanceLocation, "."); loc != "" {
			msg = loc + ": " + msg
		}
		return []string{msg}
	}
	var out []string
	for _, c := range e.Causes {
		out = append(out, problems(c)...)
	}
	switch e.ErrorKind.(type) {
	case *kind.OneOf, *kind.AnyOf:
		return []string{strings.Join(out, " or ")}
	}
	return out
}
