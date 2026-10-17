// Command gen writes core/place/subdivisions.tsv, the built-in ISO 3166-2
// subdivisions, from a pinned Unicode CLDR release and the amendments in
// amendments.tsv.
//
// It reads three CLDR files: common/validity/subdivision.xml for the ids CLDR
// marks regular, common/supplemental/subdivisions.xml for which country or
// subdivision holds each one, and common/subdivisions/en.xml for the English
// names. It then applies each amendment, a change to ISO 3166-2 the pinned
// release does not record yet, and refuses to run when the release already
// holds an amendment, so a bump of the release drops that line rather than
// applying it twice.
//
// Run it with `go generate ./core/place` from the repo root. With -cldr it
// reads the three files from a local directory laid out like the CLDR tree
// instead of downloading them.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// Release is the pinned CLDR release tag.
const Release = "release-48-2"

const baseURL = "https://raw.githubusercontent.com/unicode-org/cldr/"

var files = struct{ validity, containment, names string }{
	validity:    "common/validity/subdivision.xml",
	containment: "common/supplemental/subdivisions.xml",
	names:       "common/subdivisions/en.xml",
}

func main() {
	out := flag.String("out", "subdivisions.tsv", "file to write")
	amendments := flag.String("amendments", filepath.Join("internal", "gen", "amendments.tsv"), "amendments to apply over the release")
	local := flag.String("cldr", "", "read the CLDR files from this directory instead of downloading them")
	flag.Parse()
	if err := run(*out, *amendments, *local); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run(out, amendmentsPath, local string) error {
	read := func(rel string) ([]byte, error) {
		if local != "" {
			return os.ReadFile(filepath.Join(local, filepath.FromSlash(rel)))
		}
		return fetch(baseURL + Release + "/" + rel)
	}
	validity, err := read(files.validity)
	if err != nil {
		return err
	}
	containment, err := read(files.containment)
	if err != nil {
		return err
	}
	names, err := read(files.names)
	if err != nil {
		return err
	}
	regular, err := parseValidity(validity)
	if err != nil {
		return err
	}
	parents, err := parseContainment(containment)
	if err != nil {
		return err
	}
	english, err := parseNames(names)
	if err != nil {
		return err
	}
	rows, err := build(regular, parents, english)
	if err != nil {
		return err
	}
	amend, err := os.ReadFile(amendmentsPath)
	if err != nil {
		return err
	}
	rows, err = applyAmendments(rows, amend)
	if err != nil {
		return err
	}
	return os.WriteFile(out, render(rows), 0o644)
}

func fetch(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// row is one subdivision as the data file holds it.
type row struct{ id, parent, name string }

// parseValidity returns the CLDR subdivision codes ("no03", "frara") whose
// status is regular, expanding the release's ranges ("ad02~8").
func parseValidity(data []byte) (map[string]bool, error) {
	var doc struct {
		IDs []struct {
			Type   string `xml:"type,attr"`
			Status string `xml:"idStatus,attr"`
			Text   string `xml:",chardata"`
		} `xml:"idValidity>id"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", files.validity, err)
	}
	out := map[string]bool{}
	for _, id := range doc.IDs {
		if id.Type != "subdivision" || id.Status != "regular" {
			continue
		}
		for field := range strings.FieldsSeq(id.Text) {
			codes, err := expandRange(field)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", files.validity, err)
			}
			for _, c := range codes {
				out[c] = true
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no regular subdivision ids", files.validity)
	}
	return out, nil
}

// expandRange expands a CLDR validity range: "ad02~8" is ad02 to ad08, the
// characters after the tilde replacing the same number of trailing characters
// and each position running from its start to its end.
func expandRange(field string) ([]string, error) {
	start, end, ok := strings.Cut(field, "~")
	if !ok {
		return []string{field}, nil
	}
	if end == "" || len(end) > len(start) {
		return nil, fmt.Errorf("range %q is malformed", field)
	}
	prefix := start[:len(start)-len(end)]
	from := start[len(start)-len(end):]
	out := []string{prefix}
	for i := range len(end) {
		lo, hi := from[i], end[i]
		if hi < lo {
			return nil, fmt.Errorf("range %q runs backwards", field)
		}
		var next []string
		for _, p := range out {
			for c := lo; c <= hi; c++ {
				next = append(next, p+string(c))
			}
		}
		out = next
	}
	return out, nil
}

// parseContainment returns each CLDR subdivision code's container: a country
// code in lower case ("no") or another subdivision code ("frara").
func parseContainment(data []byte) (map[string]string, error) {
	var doc struct {
		Groups []struct {
			Type     string `xml:"type,attr"`
			Contains string `xml:"contains,attr"`
		} `xml:"subdivisionContainment>subgroup"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", files.containment, err)
	}
	out := map[string]string{}
	for _, g := range doc.Groups {
		parent := strings.ToLower(g.Type)
		for child := range strings.FieldsSeq(g.Contains) {
			if prev, dup := out[child]; dup && prev != parent {
				return nil, fmt.Errorf("%s: %s sits in both %s and %s", files.containment, child, prev, parent)
			}
			out[child] = parent
		}
	}
	return out, nil
}

// parseNames returns the English name of each CLDR subdivision code.
func parseNames(data []byte) (map[string]string, error) {
	var doc struct {
		Names []struct {
			Type string `xml:"type,attr"`
			Alt  string `xml:"alt,attr"`
			Name string `xml:",chardata"`
		} `xml:"localeDisplayNames>subdivisions>subdivision"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", files.names, err)
	}
	out := map[string]string{}
	for _, n := range doc.Names {
		if n.Alt != "" {
			continue
		}
		out[n.Type] = strings.TrimSpace(n.Name)
	}
	return out, nil
}

var codePattern = regexp.MustCompile(`^[a-z]{2}[a-z0-9]{1,4}$`)

// placeID is the gazetteer id of a CLDR subdivision code: the country code, a
// hyphen and the rest in lower case ("no03" is "no-03").
func placeID(code string) string { return code[:2] + "-" + code[2:] }

// build joins the three files into rows: every regular code that sits in a
// country, with its nearest regular container and its English name.
func build(regular map[string]bool, parents, names map[string]string) ([]row, error) {
	var rows []row
	var problems []error
	for code := range regular {
		if !codePattern.MatchString(code) {
			problems = append(problems, fmt.Errorf("%q is not a subdivision code", code))
			continue
		}
		parent, ok := parents[code]
		if !ok {
			continue // a regular code CLDR places nowhere holds no ground to draw
		}
		for len(parent) > 2 && !regular[parent] {
			next, ok := parents[parent]
			if !ok {
				problems = append(problems, fmt.Errorf("%s sits in %s, which sits nowhere", code, parent))
				break
			}
			parent = next
		}
		if len(parent) > 2 {
			parent = placeID(parent)
		}
		name := names[code]
		if name == "" {
			problems = append(problems, fmt.Errorf("%s has no English name in %s", code, files.names))
			continue
		}
		rows = append(rows, row{id: placeID(code), parent: parent, name: name})
	}
	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}
	sortRows(rows)
	return rows, nil
}

// applyAmendments applies amendments.tsv: tab-separated lines of
// `add <id> <parent> <name> <source>` and `retire <id> - - <source>`, with #
// comments and blank lines skipped. An add of an id the release already holds,
// and a retire of an id it does not, is an error.
func applyAmendments(rows []row, data []byte) ([]row, error) {
	byID := map[string]int{}
	for i, r := range rows {
		byID[r.id] = i
	}
	retired := map[string]bool{}
	var added []row
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 5 || f[4] == "" {
			return nil, fmt.Errorf("amendments line %d: want op, id, parent, name and source, tab separated", n)
		}
		op, id, parent, name := f[0], f[1], f[2], f[3]
		_, held := byID[id]
		switch op {
		case "add":
			if held {
				return nil, fmt.Errorf("amendments line %d: CLDR %s already holds %s; remove the amendment", n, Release, id)
			}
			added = append(added, row{id: id, parent: parent, name: name})
		case "retire":
			if !held {
				return nil, fmt.Errorf("amendments line %d: CLDR %s no longer holds %s; remove the amendment", n, Release, id)
			}
			retired[id] = true
		default:
			return nil, fmt.Errorf("amendments line %d: %q is neither add nor retire", n, op)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := slices.DeleteFunc(rows, func(r row) bool { return retired[r.id] })
	for _, r := range out {
		if retired[r.parent] {
			return nil, fmt.Errorf("amendments retire %s, which still holds %s", r.parent, r.id)
		}
	}
	out = append(out, added...)
	sortRows(out)
	return out, nil
}

func sortRows(rows []row) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
}

func render(rows []row) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Code generated by core/place/internal/gen from Unicode CLDR %s and internal/gen/amendments.tsv. DO NOT EDIT.\n", Release)
	b.WriteString("# id\tparent\tname\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", r.id, r.parent, r.name)
	}
	return b.Bytes()
}
