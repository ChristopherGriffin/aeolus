// Package radio knows what Wi-Fi radios can do: the widths each band allows,
// the htmodes that express them, and the 5 GHz channels that can carry each
// width. The render check, the config checks and the hardware offered on a
// folder all read it, so they always agree.
package radio

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Widths are the channel widths, in MHz, the schema allows on each band.
var Widths = map[string][]int{
	"2g": {20, 40},
	"5g": {20, 40, 80, 160},
	"6g": {20, 40, 80, 160, 320},
}

// Bands in the order people read them.
var Bands = []string{"2g", "5g", "6g"}

// ModeWidth reads the width of an htmode such as HT40, VHT160 or HE80.
// VHT80+80 and NOHT are not widths Aeolus asks for.
func ModeWidth(mode string) (int, bool) {
	for _, fam := range []string{"EHT", "HE", "VHT", "HT"} {
		rest, ok := strings.CutPrefix(mode, fam)
		if !ok {
			continue
		}
		w, err := strconv.Atoi(rest)
		return w, err == nil && w > 0
	}
	return 0, false
}

// Can lists the widths a radio can use, from the htmodes it reported.
func Can(htmodes []string) map[int]bool {
	out := map[int]bool{}
	for _, m := range htmodes {
		if w, ok := ModeWidth(m); ok {
			out[w] = true
		}
	}
	return out
}

// groups5g are the 5 GHz channels that can be joined for each width, by the
// lowest and highest 20 MHz channel in the group. They hold in the US and EU;
// the newer 165–177 groups are left out, so a radio is never asked for one
// its region may not allow.
var groups5g = map[int][][2]int{
	40:  {{36, 40}, {44, 48}, {52, 56}, {60, 64}, {100, 104}, {108, 112}, {116, 120}, {124, 128}, {132, 136}, {140, 144}, {149, 153}, {157, 161}},
	80:  {{36, 48}, {52, 64}, {100, 112}, {116, 128}, {132, 144}, {149, 161}},
	160: {{36, 64}, {100, 128}},
}

// Fits says whether a channel can carry a width on a band, and if not, why.
// Only 5 GHz is checked; channel 0 means automatic, which always fits.
func Fits(band string, channel, width int) (bool, string) {
	if band != "5g" || channel == 0 || width <= 20 {
		return true, ""
	}
	groups, ok := groups5g[width]
	if !ok {
		return false, fmt.Sprintf("a %d MHz width is not possible on 5 GHz", width)
	}
	var names []string
	for _, g := range groups {
		if channel >= g[0] && channel <= g[1] {
			return true, ""
		}
		names = append(names, fmt.Sprintf("%d–%d", g[0], g[1]))
	}
	if width == 160 {
		return false, fmt.Sprintf("channel %d cannot use a 160 MHz width; that needs a channel from %s", channel, strings.Join(names, " or "))
	}
	return false, fmt.Sprintf("channel %d cannot use a %d MHz width", channel, width)
}

// Radar says whether a radio on a channel, at a width, uses any of the 5 GHz
// channels shared with radar in the US and EU (DFS), 52–144. Such a radio
// listens for radar before it transmits and moves off if it hears any (0045).
// On channel 0, automatic, it says whether that holds whatever the radio
// picks: every block that can carry the width includes radar channels.
func Radar(band string, channel, width int) bool {
	if band != "5g" {
		return false
	}
	groups := groups5g[width]
	if channel == 0 {
		if len(groups) == 0 {
			return false // 20 MHz: it may pick a channel without radar
		}
		for _, g := range groups {
			if !radar(g[0], g[1]) {
				return false
			}
		}
		return true
	}
	for _, g := range groups {
		if channel >= g[0] && channel <= g[1] {
			return radar(g[0], g[1])
		}
	}
	return radar(channel, channel)
}

func radar(lo, hi int) bool { return lo <= 144 && hi >= 52 }

// Whole keeps, of a set of channels an automatic channel may be (0075), the
// ones a radio at width can use: on 5 GHz at 40 MHz or more, those of the
// blocks wholly in the set, as hostapd checks only a block's primary
// channel against its list. Elsewhere, the set itself. With radar, blocks
// shared with radar are dropped too. Sorted, without repeats.
func Whole(band string, set []int, width int, avoidRadar bool) []int {
	in := map[int]bool{}
	for _, c := range set {
		in[c] = true
	}
	var out []int
	if band != "5g" || width <= 20 {
		for c := range in {
			if !avoidRadar || band != "5g" || !radar(c, c) {
				out = append(out, c)
			}
		}
		slices.Sort(out)
		return out
	}
	for _, g := range groups5g[width] {
		if avoidRadar && radar(g[0], g[1]) {
			continue
		}
		all := true
		for c := g[0]; c <= g[1]; c += 4 {
			all = all && in[c]
		}
		if all {
			for c := g[0]; c <= g[1]; c += 4 {
				out = append(out, c)
			}
		}
	}
	return out
}
