// Package oui names the maker of a MAC from its first three bytes (0067),
// by the IEEE's registry of MA-L assignments, taken when the manager was
// built: run `go run ./internal/oui/gen` to take it again.
package oui

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

//go:embed oui.txt.gz
var table []byte

var (
	once   sync.Once
	makers map[uint32]string
)

func load() {
	makers = map[uint32]string{}
	z, err := gzip.NewReader(bytes.NewReader(table))
	if err != nil {
		return
	}
	s := bufio.NewScanner(z)
	for s.Scan() {
		p, name, ok := strings.Cut(s.Text(), "\t")
		if n, err := strconv.ParseUint(p, 16, 32); ok && err == nil {
			makers[uint32(n)] = name
		}
	}
}

// Lookup names the maker of mac ("aa:bb:cc:dd:ee:ff"), or "" when the
// registry doesn't know its prefix. private is true for a locally
// administered MAC, such as a phone's private one, which names no maker.
func Lookup(mac string) (maker string, private bool) {
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return "", false
	}
	var p uint32
	for _, x := range parts[:3] {
		b, err := strconv.ParseUint(x, 16, 8)
		if err != nil {
			return "", false
		}
		p = p<<8 | uint32(b)
	}
	if p>>16&0x02 != 0 {
		return "", true
	}
	once.Do(load)
	return makers[p], false
}

// Size says how many prefixes the table holds.
func Size() int {
	once.Do(load)
	return len(makers)
}
