package openxml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsChartPartPath(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"word/charts/chart1.xml", true},
		{"word/charts/chart12.xml", true},
		{"ppt/charts/chart1.xml", true},
		{"xl/charts/chart1.xml", true},
		{"word/charts/_rels/chart1.xml.rels", false},
		{"word/charts/style1.xml", true}, // any .xml under charts/ qualifies
		{"word/document.xml", false},
		{"word/diagrams/data1.xml", false},
		{"word/charts/chart1.bin", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, isChartPartPath(c.name))
		})
	}
}

func TestIsDiagramDataPartPath(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"word/diagrams/data1.xml", true},
		{"word/diagrams/data12.xml", true},
		{"ppt/diagrams/data1.xml", true},
		{"xl/diagrams/data1.xml", true},
		{"word/diagrams/_rels/data1.xml.rels", false},
		{"word/diagrams/layout1.xml", false},
		{"word/diagrams/colors1.xml", false},
		{"word/diagrams/quickStyle1.xml", false},
		{"word/diagrams/drawing1.xml", false},
		{"word/document.xml", false},
		{"word/charts/chart1.xml", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, isDiagramDataPartPath(c.name))
		})
	}
}

func TestIsStructurallyEmptyDMLBlockProperties(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "empty pPr",
			raw:  "<a:pPr></a:pPr>",
			want: true,
		},
		{
			name: "pPr with empty defRPr",
			raw:  "<a:pPr><a:defRPr></a:defRPr></a:pPr>",
			want: true,
		},
		{
			name: "pPr with defRPr carrying attributes",
			raw:  `<a:pPr><a:defRPr sz="800"></a:defRPr></a:pPr>`,
			want: false,
		},
		{
			name: "pPr with defRPr carrying child element",
			raw:  `<a:pPr><a:defRPr><a:latin typeface="Arial"></a:latin></a:defRPr></a:pPr>`,
			want: false,
		},
		{
			name: "pPr with attributes on outer element",
			raw:  `<a:pPr lvl="1"></a:pPr>`,
			want: false,
		},
		{
			name: "non-pPr element",
			raw:  "<a:rPr></a:rPr>",
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, isStructurallyEmptyDMLBlockProperties(c.raw))
		})
	}
}
