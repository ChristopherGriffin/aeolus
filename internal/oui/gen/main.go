// Command gen makes oui.txt.gz, the makers of MAC prefixes the manager knows
// (0067), from the IEEE's registry of MA-L assignments: one line a prefix,
// its six hex digits, a tab, and the organization's name, sorted.
//
//	go run ./internal/oui/gen [oui.csv]
//
// Without a file it fetches https://standards-oui.ieee.org/oui/oui.csv.
package main

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const source = "https://standards-oui.ieee.org/oui/oui.csv"

var hex6 = regexp.MustCompile(`^[0-9A-F]{6}$`)

func main() {
	var in io.Reader
	if len(os.Args) > 1 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		in = f
	} else {
		resp, err := http.Get(source)
		if err != nil {
			log.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Fatalf("%s: %s", source, resp.Status)
		}
		in = resp.Body
	}
	r := csv.NewReader(in)
	r.FieldsPerRecord = -1
	makers := map[string]string{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		if len(rec) < 3 || rec[0] != "MA-L" || !hex6.MatchString(rec[1]) {
			continue
		}
		name := strings.Join(strings.Fields(rec[2]), " ")
		if name != "" {
			makers[rec[1]] = name
		}
	}
	if len(makers) < 10000 {
		log.Fatalf("only %d prefixes: not the registry", len(makers))
	}
	prefixes := make([]string, 0, len(makers))
	for p := range makers {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	out, err := os.Create(filepath.Join("internal", "oui", "oui.txt.gz"))
	if err != nil {
		log.Fatal(err)
	}
	z, _ := gzip.NewWriterLevel(out, gzip.BestCompression)
	for _, p := range prefixes {
		fmt.Fprintf(z, "%s\t%s\n", p, makers[p])
	}
	if err := z.Close(); err != nil {
		log.Fatal(err)
	}
	if err := out.Close(); err != nil {
		log.Fatal(err)
	}
	log.Printf("%d prefixes", len(prefixes))
}
