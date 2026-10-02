package radio

import (
	"reflect"
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
