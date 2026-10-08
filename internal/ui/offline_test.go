package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Nothing Aeolus runs needs the internet (0069): the UI and the agent name
// no internet address. The few that are named are no place they fetch from.
var named = map[string]string{
	"http://www.w3.org/2000/svg":              "the SVG namespace, a name and not a place",
	"https://downloads.openwrt.org/releases/": "OpenWrt's feeds, named only to point them at the manager",
	"https://aeolus.symtus.com:8443":          "a manager, in install.sh's example",
	"https://192.168.20.60:8443":              "a manager by its private address, as 0040 advises, in aeolus-setup's and install.sh's examples (0080)",
}

var address = regexp.MustCompile(`https?://[A-Za-z0-9.\-]+(:[0-9]+)?(/[A-Za-z0-9._~/\-]*)?`)

func TestNoInternetAddresses(t *testing.T) {
	roots := []string{"static", "../../agent/files", "../../agent/install.sh"}
	seen := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			seen++
			for _, a := range address.FindAllString(string(b), -1) {
				if _, ok := named[a]; !ok {
					t.Errorf("%s names %s: nothing Aeolus runs may fetch from the internet (0069)", p, a)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if seen < 10 {
		t.Fatalf("looked at only %d files", seen)
	}
}
