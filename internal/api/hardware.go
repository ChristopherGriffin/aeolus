package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ChristopherGriffin/aeolus/internal/change"
	"github.com/ChristopherGriffin/aeolus/internal/hierarchy"
	"github.com/ChristopherGriffin/aeolus/internal/radio"
)

// What a Locations node can offer for its radios (0044). A folder offers
// only what every AP it reaches can do, so its setting is the same for all
// of them; an AP offers what its own radios can do, as its own setting.

type apRef struct {
	ID   hierarchy.NodeID `json:"id"`
	Name string           `json:"name"`
}

type widthView struct {
	Width int    `json:"width"`
	OK    bool   `json:"ok"`
	Why   string `json:"why,omitempty"`
}

type bandView struct {
	Band   string      `json:"band"`
	APs    int         `json:"aps"`
	Widths []widthView `json:"widths"`
}

type hardwareView struct {
	APs     []apRef    `json:"aps"`
	Unknown []apRef    `json:"unknown"` // never said what radios they have
	Bands   []bandView `json:"bands"`
}

// apRadios is what one AP said about its radios when it enrolled (0033),
// with the channel each band is on: the one Aeolus sets, or else the one it
// last reported.
type apRadios struct {
	ref      apRef
	can      map[string]map[int]bool // band -> widths every radio of that band can use
	channels map[string]int          // band -> channel; 0 for automatic or unknown
}

// reach lists the APs a setting on node would apply to: node itself if it is
// an AP, or every AP below it, except below a break or in an isolated folder,
// where inheritance from above stops (0005, 0032).
func reach(t *hierarchy.Tree, node hierarchy.NodeID) []hierarchy.NodeID {
	n, ok := t.Node(node)
	if !ok {
		return nil
	}
	if n.Kind == hierarchy.KindAP {
		return []hierarchy.NodeID{node}
	}
	var out []hierarchy.NodeID
	var walk func(hierarchy.NodeID)
	walk = func(id hierarchy.NodeID) {
		for _, c := range t.Children(id) {
			cn, _ := t.Node(c)
			switch {
			case cn.Kind == hierarchy.KindAP:
				out = append(out, c)
			case !cn.Broken && !cn.Isolated:
				walk(c)
			}
		}
	}
	walk(node)
	return out
}

func (s *Server) hardware(state *change.State, node hierarchy.NodeID) (*hardwareView, error) {
	t := state.Org.Locations
	self, _ := t.Node(node)
	hw := &hardwareView{APs: []apRef{}, Unknown: []apRef{}, Bands: []bandView{}}
	var known []apRadios
	for _, id := range reach(t, node) {
		n, _ := t.Node(id)
		ref := apRef{ID: id, Name: n.Name}
		hw.APs = append(hw.APs, ref)
		r, err := s.apRadios(state, t, ref)
		if err != nil {
			return nil, err
		}
		if len(r.can) == 0 {
			hw.Unknown = append(hw.Unknown, ref)
			continue
		}
		known = append(known, r)
	}
	alone := self.Kind == hierarchy.KindAP
	for _, band := range radio.Bands {
		var have []apRadios
		for _, r := range known {
			if r.can[band] != nil {
				have = append(have, r)
			}
		}
		if len(have) == 0 {
			continue
		}
		bv := bandView{Band: band, APs: len(have), Widths: []widthView{}}
		for _, w := range radio.Widths[band] {
			bv.Widths = append(bv.Widths, judge(band, w, have, alone))
		}
		hw.Bands = append(hw.Bands, bv)
	}
	return hw, nil
}

// judge says whether every AP can use a width on a band, and if not, the
// first reason and how many more APs share the trouble.
func judge(band string, w int, aps []apRadios, alone bool) widthView {
	var cannot []string
	var onChannel []apRadios
	for _, r := range aps {
		if !r.can[band][w] {
			cannot = append(cannot, r.ref.Name)
		} else if ok, _ := radio.Fits(band, r.channels[band], w); !ok {
			onChannel = append(onChannel, r)
		}
	}
	switch {
	case len(cannot) > 0 && alone:
		return widthView{Width: w, Why: "its radio cannot do it"}
	case len(cannot) > 0:
		return widthView{Width: w, Why: names(cannot) + " cannot do it"}
	case len(onChannel) > 0 && alone:
		return widthView{Width: w, Why: fmt.Sprintf("not on channel %d", onChannel[0].channels[band])}
	case len(onChannel) > 0:
		first := onChannel[0]
		return widthView{Width: w, Why: fmt.Sprintf("%s is on channel %d", first.ref.Name, first.channels[band]) + more(len(onChannel)-1)}
	}
	return widthView{Width: w, OK: true}
}

func names(list []string) string {
	if len(list) <= 2 {
		return strings.Join(list, " and ")
	}
	return list[0] + more(len(list)-1)
}

func more(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" (and %d more)", n)
}

func (s *Server) apRadios(state *change.State, t *hierarchy.Tree, ref apRef) (apRadios, error) {
	r := apRadios{ref: ref, can: map[string]map[int]bool{}, channels: map[string]int{}}
	var facts struct {
		Radios []struct {
			Band    string   `json:"band"`
			HTModes []string `json:"htmodes"`
		} `json:"radios"`
	}
	if raw := state.Facts[ref.ID]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &facts); err != nil {
			return r, nil // facts we cannot read are facts we do not have
		}
	}
	for _, fr := range facts.Radios {
		can := radio.Can(fr.HTModes)
		if len(can) == 0 {
			continue
		}
		if prev, ok := r.can[fr.Band]; ok {
			for w := range prev { // a band setting applies to every radio of that band
				if !can[w] {
					delete(prev, w)
				}
			}
		} else {
			r.can[fr.Band] = can
		}
	}
	reported, err := s.reportedChannels(ref.ID)
	if err != nil {
		return r, err
	}
	for band := range r.can {
		if v, ok := t.Resolve(ref.ID, hierarchy.Path("radio."+band+".channel")); ok {
			if ch, ok := v.Value.(float64); ok {
				r.channels[band] = int(ch)
				continue
			}
			if v.Value == "auto" {
				continue
			}
		}
		r.channels[band] = reported[band]
	}
	return r, nil
}

// reportedChannels reads the channel each band was on in an AP's latest
// state report (0039).
func (s *Server) reportedChannels(ap hierarchy.NodeID) (map[string]int, error) {
	out := map[string]int{}
	l, err := s.conds.Latest(ap)
	if err != nil || l.State == nil {
		return out, err
	}
	var rep struct {
		Radios []struct {
			Band    string `json:"band"`
			Channel int    `json:"channel"`
		} `json:"radios"`
	}
	if json.Unmarshal(l.State.Report, &rep) != nil {
		return out, nil
	}
	for _, r := range rep.Radios {
		if _, seen := out[r.Band]; !seen {
			out[r.Band] = r.Channel
		}
	}
	return out, nil
}
