// Package compose builds the config an AP receives (0008): its resolved
// fields, filtered and completed with the library (0023), and checked whole
// (0029). The API shows it, and the AP poll will serve it.
package compose

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/radio"
	"github.com/ChristopherGriffin/aeolus/internal/schema"
)

// Reveal opens a sealed secret for the field it belongs to. A nil Reveal
// leaves secrets sealed.
type Reveal func(path string, v any) (any, error)

// Result is an AP's composed config.
type Result struct {
	// Unassigned: the AP is in Landing Zone and gets no config (0032).
	Unassigned bool
	// Doc is the nested config the AP receives.
	Doc map[string]any
	// Problems are the rules the config breaks; an AP whose config has
	// problems is not sent it (0029).
	Problems []string
}

var slots = []string{"primary", "fallback"}

// AP composes one AP's config:
//   - A VXLAN transport whose concentrator may not be used at the AP's
//     location is left out, so the AP never tries a concentrator it cannot
//     reach. If that removes the primary, the fallback takes its place.
//   - A network left with no transport is a problem, as is a transport
//     naming a concentrator the library lacks, or a VNI that concentrator
//     does not have.
//   - The concentrators the AP's transports use are attached, with what the
//     AP needs to reach them.
//   - A radio width the AP's radio cannot do, by what it reported when it
//     enrolled, is a problem (0008).
func AP(s *change.State, sch *schema.Schema, ap hierarchy.NodeID, reveal Reveal) (Result, error) {
	cfg, err := s.Org.ResolveAP(ap)
	if err != nil {
		return Result{}, err
	}
	if cfg.Unassigned {
		return Result{Unassigned: true, Problems: []string{}}, nil
	}
	ancestry := s.Org.Locations.Ancestry(ap)
	fields := map[string]any{}
	for p, r := range cfg.Location {
		fields[string(p)] = r.Value
	}
	var problems []string
	used := map[string]bool{}

	ids := cfg.NetworkIDs()
	for _, id := range ids {
		net := map[string]any{}
		for f, r := range cfg.Networks[id].Fields {
			net[f] = r.Value
		}
		kept := map[string]bool{}
		for _, slot := range slots {
			prefix := "transport." + slot + "."
			if _, has := net[prefix+"type"]; !has {
				continue
			}
			if net[prefix+"type"] != "vxlan" {
				kept[slot] = true
				continue
			}
			cid, _ := net[prefix+"concentrator"].(string)
			k, ok := s.Library.Get(cid)
			switch {
			case !ok:
				problems = append(problems, fmt.Sprintf("network.%s.transport.%s: concentrator %q is not in the library", id, slot, cid))
			case !k.AvailableAt(ancestry):
				// Not usable here (0023): leave it out.
			default:
				if vni, ok := net[prefix+"vni"].(float64); ok {
					if _, defined := k.VNIs[int(vni)]; !defined {
						problems = append(problems, fmt.Sprintf("network.%s.transport.%s: VNI %d is not defined on concentrator %s", id, slot, int(vni), cid))
					}
				}
				kept[slot] = true
				used[cid] = true
			}
		}
		hadTransport := false
		for _, slot := range slots {
			if _, has := net["transport."+slot+".type"]; has {
				hadTransport = true
			}
			if !kept[slot] {
				drop(net, "transport."+slot+".")
			}
		}
		if !kept["primary"] && kept["fallback"] {
			promote(net)
		}
		if hadTransport && !kept["primary"] && !kept["fallback"] {
			problems = append(problems, fmt.Sprintf("network.%s: no transport is usable at this AP", id))
		}
		for f, v := range net {
			fields["network."+id+"."+f] = v
		}
	}

	doc, err := schema.Assemble(fields, reveal)
	if err != nil {
		return Result{}, err
	}
	if len(used) > 0 {
		concs := map[string]any{}
		for cid := range used {
			k, _ := s.Library.Get(cid)
			concs[cid] = map[string]any{"address": k.Address, "port": k.Port, "mtu": k.MTU}
		}
		doc["concentrators"] = concs
	}
	doc = jsonShape(doc)
	sort.Strings(problems)
	problems = append(problems, sch.Problems(doc)...)
	problems = append(problems, radioProblems(doc, s.Facts[ap])...)
	if problems == nil {
		problems = []string{}
	}
	return Result{Doc: doc, Problems: problems}, nil
}

// radioProblems refuses a width a radio cannot do, by the modes the AP
// reported for it when it enrolled (0008). A band the AP did not report, or
// reported without modes, is not checked.
func radioProblems(doc map[string]any, facts json.RawMessage) []string {
	var f struct {
		Radios []struct {
			Band    string   `json:"band"`
			HTModes []string `json:"htmodes"`
		} `json:"radios"`
	}
	if len(facts) == 0 || json.Unmarshal(facts, &f) != nil {
		return nil
	}
	widths := map[string]map[int]bool{} // band -> widths some radio of it can do
	for _, r := range f.Radios {
		if widths[r.Band] == nil {
			widths[r.Band] = map[int]bool{}
		}
		for w := range radio.Can(r.HTModes) {
			widths[r.Band][w] = true
		}
	}
	radios, _ := doc["radio"].(map[string]any)
	var out []string
	for _, band := range sortedKeys(radios) {
		set, _ := radios[band].(map[string]any)
		w, ok := set["width"].(float64)
		can := widths[band]
		if !ok || len(can) == 0 || can[int(w)] {
			continue
		}
		var list []string
		for _, x := range []int{20, 40, 80, 160, 320} {
			if can[x] {
				list = append(list, fmt.Sprint(x))
			}
		}
		out = append(out, fmt.Sprintf("radio.%s.width: this AP's radio cannot use %d MHz; it can use %s MHz", band, int(w), strings.Join(list, ", ")))
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Node checks what is resolved at one node as a partial config: the schema's
// rules, and, in the Services tree, that each transport's concentrator is in
// the library and has the VNI. The page guard uses it (0029).
func Node(s *change.State, sch *schema.Schema, tree change.TreeName, t *hierarchy.Tree, node hierarchy.NodeID, reveal Reveal) []string {
	values := map[string]any{}
	for p, r := range t.ResolveAll(node) {
		if p != hierarchy.ServicesPath {
			values[string(p)] = r.Value
		}
	}
	var problems []string
	if tree == change.Services {
		for p, v := range values {
			network, slot, field, ok := change.TransportField(hierarchy.Path(p))
			if !ok || field != "concentrator" {
				continue
			}
			cid, _ := v.(string)
			k, ok := s.Library.Get(cid)
			if !ok {
				problems = append(problems, fmt.Sprintf("network.%s.transport.%s: concentrator %q is not in the library", network, slot, cid))
				continue
			}
			if vni, ok := values["network."+network+".transport."+slot+".vni"].(float64); ok {
				if _, defined := k.VNIs[int(vni)]; !defined {
					problems = append(problems, fmt.Sprintf("network.%s.transport.%s: VNI %d is not defined on concentrator %s", network, slot, int(vni), cid))
				}
			}
		}
	}
	sort.Strings(problems)
	doc, err := schema.Assemble(values, reveal)
	if err != nil {
		return append(problems, err.Error())
	}
	problems = append(problems, sch.Problems(jsonShape(doc))...)
	if problems == nil {
		problems = []string{}
	}
	return problems
}

// jsonShape passes a document through JSON so it holds only JSON types, as the
// AP will receive it.
func jsonShape(doc map[string]any) map[string]any {
	raw, err := json.Marshal(doc)
	if err != nil {
		return doc
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return doc
	}
	return out
}

func drop(net map[string]any, prefix string) {
	for f := range net {
		if strings.HasPrefix(f, prefix) {
			delete(net, f)
		}
	}
}

// promote moves the fallback transport into the primary's place.
func promote(net map[string]any) {
	for f, v := range net {
		if rest, ok := strings.CutPrefix(f, "transport.fallback."); ok {
			net["transport.primary."+rest] = v
			delete(net, f)
		}
	}
}
