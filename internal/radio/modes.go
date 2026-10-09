package radio

import (
	"fmt"
	"slices"
	"strings"
)

// The 802.11 generations a band's clients may use (0089), and what each
// takes on an AP. A band's list is one unbroken run, oldest to newest: the
// oldest is the least a client must support, OpenWrt's require_mode, and
// the newest caps the htmode family the radio runs. 802.11b is OpenWrt's
// legacy_rates, which is off unless set.

// Generations are each band's, oldest first.
var Generations = map[string][]string{
	"2g": {"b", "g", "n", "ax", "be"},
	"5g": {"a", "n", "ac", "ax", "be"},
	"6g": {"ax", "be"},
}

// Family is the htmode family that serves a generation, and NOHT for those
// before 802.11n.
var Family = map[string]string{"b": "NOHT", "g": "NOHT", "a": "NOHT", "n": "HT", "ac": "VHT", "ax": "HE", "be": "EHT"}

// Families in order, oldest first.
var Families = []string{"NOHT", "HT", "VHT", "HE", "EHT"}

// FamilyWidth is the widest channel each family takes, in MHz.
var FamilyWidth = map[string]int{"NOHT": 20, "HT": 40, "VHT": 160, "HE": 160, "EHT": 320}

// Required is OpenWrt's require_mode for the oldest generation allowed. b,
// g and a need none; OpenWrt has no way to require 802.11be, and on 6 GHz
// every client is 802.11ax already.
func Required(band, oldest string) string {
	if band == "6g" {
		return ""
	}
	return map[string]string{"n": "n", "ac": "ac", "ax": "ax"}[oldest]
}

// Span reads a band's list of generations: each one the band has, without
// a gap. It returns the oldest and the newest.
func Span(band string, modes []string) (oldest, newest string, err error) {
	order := Generations[band]
	var at []int
	for _, m := range modes {
		i := slices.Index(order, m)
		if i < 0 {
			return "", "", fmt.Errorf("%s has no 802.11%s", bandName(band), m)
		}
		at = append(at, i)
	}
	if len(at) == 0 {
		return "", "", fmt.Errorf("no 802.11 generation is allowed")
	}
	slices.Sort(at)
	at = slices.Compact(at)
	for i := 1; i < len(at); i++ {
		if at[i] != at[i-1]+1 {
			return "", "", fmt.Errorf("802.11%s and 802.11%s are allowed, but not 802.11%s between them", order[at[i-1]], order[at[i]], order[at[i-1]+1])
		}
	}
	return order[at[0]], order[at[len(at)-1]], nil
}

// Rank orders htmode families, NOHT first; -1 for one it doesn't know.
func Rank(family string) int {
	return slices.Index(Families, family)
}

// Best is the newest htmode family among a radio's htmodes, or "" where it
// reported none.
func Best(htmodes []string) string {
	best := ""
	for _, m := range htmodes {
		for _, fam := range []string{"EHT", "HE", "VHT", "HT"} {
			if strings.HasPrefix(m, fam) && Rank(fam) > Rank(best) {
				best = fam
			}
		}
	}
	return best
}

// Serves lists the generations a radio can serve on a band, from the
// htmodes it reported: those before 802.11n always, and the rest up to its
// newest family. nil where it reported none.
func Serves(band string, htmodes []string) []string {
	best := Best(htmodes)
	if best == "" {
		return nil
	}
	var out []string
	for _, g := range Generations[band] {
		if Rank(Family[g]) <= Rank(best) {
			out = append(out, g)
		}
	}
	return out
}

// GI is what iw takes for an 802.11ax guard interval in nanoseconds
// (0089): 0.8, 1.6 or 3.2 microseconds.
var GI = map[int]string{800: "0.8", 1600: "1.6", 3200: "3.2"}

// BandName is a band as people read it: 2.4 GHz, 5 GHz or 6 GHz.
func BandName(band string) string { return bandName(band) }
