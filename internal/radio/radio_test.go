package radio

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestModeWidth(t *testing.T) {
	for m, want := range map[string]int{"HT20": 20, "HT40": 40, "VHT80": 80, "VHT160": 160, "HE80": 80, "EHT320": 320, "VHT80+80": 0, "NOHT": 0, "": 0} {
		got, ok := ModeWidth(m)
		if got != want || ok != (want > 0) {
			t.Errorf("ModeWidth(%q) = %d, %v", m, got, ok)
		}
	}
	if got := Can([]string{"HT20", "HT40", "VHT80", "VHT80+80"}); !reflect.DeepEqual(got, map[int]bool{20: true, 40: true, 80: true}) {
		t.Errorf("Can = %v", got)
	}
}

func TestFits(t *testing.T) {
	cases := []struct {
		band       string
		channel, w int
		ok         bool
		why        string
	}{
		{"5g", 149, 80, true, ""},
		{"5g", 149, 160, false, "that needs a channel from 36–64 or 100–128"},
		{"5g", 36, 160, true, ""},
		{"5g", 165, 40, false, "channel 165 cannot use a 40 MHz width"},
		{"5g", 165, 20, true, ""},
		{"5g", 0, 160, true, ""},
		{"2g", 13, 40, true, ""},
	}
	for _, c := range cases {
		ok, why := Fits(c.band, c.channel, c.w)
		if ok != c.ok || !strings.Contains(why, c.why) {
			t.Errorf("Fits(%s, %d, %d) = %v %q", c.band, c.channel, c.w, ok, why)
		}
	}
}

func TestRadar(t *testing.T) {
	for _, c := range []struct {
		band       string
		channel, w int
		want       bool
	}{
		{"5g", 36, 160, true}, // 36–64 includes 52–64
		{"5g", 36, 80, false},
		{"5g", 52, 20, true},
		{"5g", 48, 40, false},
		{"5g", 100, 80, true},
		{"5g", 149, 80, false},
		{"5g", 0, 160, true}, // automatic: every 160 MHz block has radar channels
		{"5g", 0, 80, false}, // 36–48 and 149–161 do not
		{"5g", 0, 20, false},
		{"2g", 6, 40, false},
	} {
		if got := Radar(c.band, c.channel, c.w); got != c.want {
			t.Errorf("Radar(%s, %d, %d) = %v, want %v", c.band, c.channel, c.w, got, c.want)
		}
	}
}

// A set keeps only whole blocks at the width on 5 GHz (0075): hostapd
// checks only a block's primary channel against its list.
func TestWhole(t *testing.T) {
	set := []int{36, 40, 44, 48, 52, 56, 149, 153, 165}
	for _, c := range []struct {
		band  string
		width int
		avoid bool
		want  []int
	}{
		{"5g", 20, false, []int{36, 40, 44, 48, 52, 56, 149, 153, 165}},
		{"5g", 20, true, []int{36, 40, 44, 48, 149, 153, 165}},
		{"5g", 40, false, []int{36, 40, 44, 48, 52, 56, 149, 153}},
		{"5g", 40, true, []int{36, 40, 44, 48, 149, 153}},
		{"5g", 80, false, []int{36, 40, 44, 48}},
		{"5g", 160, false, nil},
		{"2g", 20, true, []int{36, 40, 44, 48, 52, 56, 149, 153, 165}},
	} {
		if got := Whole(c.band, set, c.width, c.avoid); !slices.Equal(got, c.want) {
			t.Errorf("Whole(%s, %d MHz, avoid %v) = %v, want %v", c.band, c.width, c.avoid, got, c.want)
		}
	}
	if got := Whole("2g", []int{11, 1, 6, 6}, 20, false); !slices.Equal(got, []int{1, 6, 11}) {
		t.Errorf("2.4 GHz: %v", got)
	}
}
