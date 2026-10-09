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

// groups6g are the 6 GHz channels joined at each width, 1–233 (5925–7125
// MHz), as in the US (0087): a 40 MHz block every 8, 80 every 16, 160
// every 32; 320 MHz blocks come in two families, 1–61 and 33–93 and so on,
// that overlap by 160 MHz.
var groups6g = func() map[int][][2]int {
	g := map[int][][2]int{}
	for _, w := range []struct{ width, span, last int }{{40, 8, 229}, {80, 16, 221}, {160, 32, 221}} {
		for lo := 1; lo+w.span-4 <= w.last; lo += w.span {
			g[w.width] = append(g[w.width], [2]int{lo, lo + w.span - 4})
		}
	}
	for _, lo := range []int{1, 33, 65, 97, 129, 161} {
		g[320] = append(g[320], [2]int{lo, lo + 60})
	}
	return g
}()

// Channels6 are 6 GHz's 20 MHz channels, 1–233.
var Channels6 = func() []int {
	var out []int
	for c := 1; c <= 233; c += 4 {
		out = append(out, c)
	}
	return out
}()

// PSC says whether a 6 GHz channel is a preferred scanning channel: 5, 21,
// 37 and every 16th to 229. A client looks for 6 GHz networks on these by
// itself; anywhere else, only where a neighbour report sends it (0087).
func PSC(channel int) bool {
	return channel >= 5 && channel <= 229 && (channel-5)%16 == 0
}

// PSCOnly keeps the preferred scanning channels of a list.
func PSCOnly(chans []int) []int {
	var out []int
	for _, c := range chans {
		if PSC(c) {
			out = append(out, c)
		}
	}
	return out
}

// groups are the blocks a band's channels are joined in at a width: 5 GHz's
// and 6 GHz's; none elsewhere, or at 20 MHz.
func groups(band string, width int) ([][2]int, bool) {
	switch band {
	case "5g":
		g, ok := groups5g[width]
		return g, ok
	case "6g":
		g, ok := groups6g[width]
		return g, ok
	}
	return nil, false
}

// Fits says whether a channel can carry a width on a band, and if not, why.
// 5 and 6 GHz are checked; channel 0 means automatic, which always fits.
func Fits(band string, channel, width int) (bool, string) {
	if (band != "5g" && band != "6g") || channel == 0 || width <= 20 {
		return true, ""
	}
	groups, ok := groups(band, width)
	if !ok {
		return false, fmt.Sprintf("a %d MHz width is not possible on %s", width, bandName(band))
	}
	var names []string
	for _, g := range groups {
		if channel >= g[0] && channel <= g[1] {
			return true, ""
		}
		names = append(names, fmt.Sprintf("%d–%d", g[0], g[1]))
	}
	if band == "5g" && width == 160 {
		return false, fmt.Sprintf("channel %d cannot use a 160 MHz width; that needs a channel from %s", channel, strings.Join(names, " or "))
	}
	if band == "6g" {
		return false, fmt.Sprintf("channel %d cannot use a %d MHz width on 6 GHz", channel, width)
	}
	return false, fmt.Sprintf("channel %d cannot use a %d MHz width", channel, width)
}

func bandName(band string) string {
	return map[string]string{"2g": "2.4 GHz", "5g": "5 GHz", "6g": "6 GHz"}[band]
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
// ones a radio at width can use: on 5 and 6 GHz at 40 MHz or more, those of
// the blocks wholly in the set, as hostapd checks only a block's primary
// channel against its list. Elsewhere, the set itself. With radar, blocks
// shared with radar are dropped too. Sorted, without repeats.
func Whole(band string, set []int, width int, avoidRadar bool) []int {
	in := map[int]bool{}
	for _, c := range set {
		in[c] = true
	}
	var out []int
	blocks, _ := groups(band, width)
	if len(blocks) == 0 || width <= 20 {
		for c := range in {
			if !avoidRadar || band != "5g" || !radar(c, c) {
				out = append(out, c)
			}
		}
		slices.Sort(out)
		return out
	}
	for _, g := range blocks {
		if avoidRadar && band == "5g" && radar(g[0], g[1]) {
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
	slices.Sort(out)
	return slices.Compact(out) // 6 GHz's 320 MHz blocks overlap
}

// SixGHzEncryption is the UCI encryption a network's security takes on
// 6 GHz, and whether it may be there at all (0086). 6 GHz takes WPA3 and
// OWE only: a network in WPA2/WPA3 transition is WPA3 alone there, and one
// that is WPA2 or open is not offered there.
func SixGHzEncryption(security string) (string, bool) {
	switch security {
	case "wpa3-sae", "wpa2-wpa3":
		return "sae", true
	case "owe":
		return "owe", true
	}
	return "", false
}

// Usable is what an automatic channel on a band may be, the list hostapd is
// given (0075, 0087): the channels of the set's whole blocks at the width,
// outside radar where it is avoided; on 6 GHz, with psc, only preferred
// scanning channels; and with spread, one channel to a block, each block
// apart from the others, so radios on different channels never share one.
// A spread block's channel is its first that may be used. Sorted.
func Usable(band string, set []int, width int, avoidRadar, psc, spread bool) []int {
	whole := Whole(band, set, width, avoidRadar)
	psc = psc && band == "6g" // preferred scanning channels are 6 GHz's
	ok := func(c int) bool { return !psc || PSC(c) }
	if !spread || width <= 20 {
		var out []int
		for _, c := range whole {
			if ok(c) {
				out = append(out, c)
			}
		}
		return out
	}
	in := map[int]bool{}
	for _, c := range whole {
		in[c] = true
	}
	blocks, _ := groups(band, width)
	blocks = slices.Clone(blocks)
	slices.SortFunc(blocks, func(a, b [2]int) int { return a[0] - b[0] })
	var out []int
	last := 0 // the highest channel of the blocks taken
	for _, b := range blocks {
		if b[0] <= last || !in[b[0]] || !in[b[1]] {
			continue
		}
		for c := b[0]; c <= b[1]; c += 4 {
			if ok(c) {
				out = append(out, c)
				last = b[1]
				break
			}
		}
	}
	return out
}
