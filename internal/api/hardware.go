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
// of them; an AP offers what its own radios can do, as its own setting. A
// width some AP's channel cannot carry comes with the channel to move to,
// set in the same change (0045).

type apRef struct {
	ID   hierarchy.NodeID `json:"id"`
	Name string           `json:"name"`
}

type widthView struct {
	Width   int        `json:"width"`
	OK      bool       `json:"ok"`
	Why     string     `json:"why,omitempty"`
	Channel int        `json:"channel,omitempty"` // the channel to set with this width, when some AP's cannot carry it
	Moves   []moveView `json:"moves,omitempty"`   // the APs that setting the channel moves
	Radar   bool       `json:"radar,omitempty"`   // some AP would then be on radar (DFS) channels
}

// moveView is an AP whose channel a change moves, and the channel it is on
// now: 0 for automatic or not known.
type moveView struct {
	apRef
	From int `json:"from"`
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
	setBelow map[string]apRef        // band -> the node below that sets its channel, out of the node's reach
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
	above := map[hierarchy.NodeID]bool{}
	for _, a := range t.Ancestry(node) {
		above[a] = true
	}
	hw := &hardwareView{APs: []apRef{}, Unknown: []apRef{}, Bands: []bandView{}}
	var known []apRadios
	for _, id := range reach(t, node) {
		n, _ := t.Node(id)
		ref := apRef{ID: id, Name: n.Name}
		hw.APs = append(hw.APs, ref)
		r, err := s.apRadios(state, t, ref, above)
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
		lockedBy := channelLock(t, node, band)
		for _, w := range radio.Widths[band] {
			bv.Widths = append(bv.Widths, judge(band, w, have, alone, lockedBy))
		}
		hw.Bands = append(hw.Bands, bv)
	}
	return hw, nil
}

// channelLock names the node whose lock on a band's channel stops node
// setting it, or is empty.
func channelLock(t *hierarchy.Tree, node hierarchy.NodeID, band string) string {
	r, ok := t.Resolve(node, hierarchy.Path("radio."+band+".channel"))
	if !ok || r.Origin != hierarchy.OriginLocked || r.From == node {
		return ""
	}
	n, _ := t.Node(r.From)
	return n.Name
}

// judge says whether every AP can use a width on a band, and if not, the
// first reason and how many more APs share the trouble. Where an AP's
// channel cannot carry the width, the width comes with the channel to move
// to, unless a channel set below or a lock above is in the way (0045).
func judge(band string, w int, aps []apRadios, alone bool, lockedBy string) widthView {
	var cannot []string
	var stuck []apRadios
	move := false
	for _, r := range aps {
		switch {
		case !r.can[band][w]:
			cannot = append(cannot, r.ref.Name)
		case fits(band, r.channels[band], w):
		case r.setBelow[band].ID != "":
			stuck = append(stuck, r)
		default:
			move = true
		}
	}
	switch {
	case len(cannot) > 0 && alone:
		return widthView{Width: w, Why: "its radio cannot do it"}
	case len(cannot) > 0:
		return widthView{Width: w, Why: names(cannot) + " cannot do it"}
	case len(stuck) > 0:
		first, by := stuck[0], stuck[0].setBelow[band]
		why := fmt.Sprintf("%s sets its own channel %d", first.ref.Name, first.channels[band])
		if by.ID != first.ref.ID {
			why = fmt.Sprintf("%s sets channel %d for %s", by.Name, first.channels[band], first.ref.Name)
		}
		return widthView{Width: w, Why: why + more(len(stuck)-1)}
	case move && lockedBy != "":
		return widthView{Width: w, Why: "the channel is locked at " + lockedBy}
	}
	v := widthView{Width: w, OK: true}
	if move {
		v.Channel = radio.MoveTo(band, w)
	}
	for _, r := range aps {
		ch := r.channels[band]
		if move && r.setBelow[band].ID == "" {
			if ch != v.Channel {
				v.Moves = append(v.Moves, moveView{r.ref, ch})
			}
			ch = v.Channel
		}
		v.Radar = v.Radar || radio.Radar(band, ch, w)
	}
	return v
}

func fits(band string, channel, w int) bool {
	ok, _ := radio.Fits(band, channel, w)
	return ok
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

// apRadios reads one AP's radios. above holds the node the setting would be
// made on and its ancestors: a channel set anywhere else is set below it.
func (s *Server) apRadios(state *change.State, t *hierarchy.Tree, ref apRef, above map[hierarchy.NodeID]bool) (apRadios, error) {
	r := apRadios{ref: ref, can: map[string]map[int]bool{}, channels: map[string]int{}, setBelow: map[string]apRef{}}
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
			if !above[v.From] {
				n, _ := t.Node(v.From)
				r.setBelow[band] = apRef{ID: v.From, Name: n.Name}
			}
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
