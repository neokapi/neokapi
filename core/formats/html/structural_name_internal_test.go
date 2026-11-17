package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/neokapi/neokapi/core/model"
)

// referencePath names a node by counting its preceding siblings directly. It is
// the definition domNamer.path must agree with.
func referencePath(n *html.Node) string {
	var steps []string
	for cur := n; cur != nil && cur.Type != html.DocumentNode; cur = cur.Parent {
		if cur != n && cur.Type == html.ElementNode && implicitStructureTags[cur.DataAtom] {
			continue
		}
		var step string
		switch {
		case cur.Type == html.ElementNode && getAttr(cur, "id") != "":
			step = idStep(getAttr(cur, "id"))
		default:
			ordinal := 1
			for sib := cur.PrevSibling; sib != nil; sib = sib.PrevSibling {
				if sib.Type == cur.Type && (cur.Type == html.TextNode || sib.Data == cur.Data) {
					ordinal++
				}
			}
			name := cur.Data
			if cur.Type == html.TextNode {
				name = textStepName
			}
			step = ordinalStep(name, ordinal)
		}
		steps = append(steps, step)
		if cur.Type == html.ElementNode && getAttr(cur, "id") != "" {
			break
		}
	}
	reverse(steps)
	return model.StructuralPath(steps...)
}

func namedNodes(doc *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode || n.Type == html.TextNode {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

// TestDOMNamerMatchesSiblingCount names every element and text node of the
// HTML test documents, plus a document mixing text, comments, ids and repeated
// tags, and checks each name against the direct sibling count.
func TestDOMNamerMatchesSiblingCount(t *testing.T) {
	inputs := map[string]string{
		"mixed": `<html><body>a<p>one</p><!-- c --><p id="x">two</p>b<div><p>three` +
			`</p><span>s</span><p>four</p>c<span>t</span></div><p>five</p>d</body></html>`,
	}
	files, err := filepath.Glob("testdata/*.html")
	require.NoError(t, err)
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		inputs[filepath.Base(f)] = string(data)
	}

	for name, src := range inputs {
		t.Run(name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(src))
			require.NoError(t, err)
			names := &domNamer{}
			for _, n := range namedNodes(doc) {
				want := referencePath(n)
				assert.Equal(t, want, names.path(n))
			}
		})
	}
}

// TestDOMNamerScansEachParentOnce guards the cost of naming a flat document:
// naming every child of a parent with many children must read that parent's
// child list once, not once per child.
func TestDOMNamerScansEachParentOnce(t *testing.T) {
	const paragraphs = 2000
	var b strings.Builder
	b.WriteString("<html><body>")
	for range paragraphs {
		b.WriteString("<p>text</p>")
	}
	b.WriteString("</body></html>")
	doc, err := html.Parse(strings.NewReader(b.String()))
	require.NoError(t, err)

	names := &domNamer{}
	parents := map[*html.Node]bool{}
	var last string
	for _, n := range namedNodes(doc) {
		last = names.path(n)
		for cur := n; cur != nil && cur.Type != html.DocumentNode; cur = cur.Parent {
			if cur.Parent != nil {
				parents[cur.Parent] = true
			}
		}
	}
	assert.Equal(t, "p#2000/text()", last)
	assert.Equal(t, len(parents), names.scans,
		"each parent's children are numbered in a single pass")
}
