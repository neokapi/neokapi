//go:build integration

package compat

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeHTML_Charset(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"meta charset attribute",
			`<meta charset="ISO-8859-1">`,
			`<meta charset="UTF-8">`,
		},
		{
			"content-type charset",
			`<meta http-equiv="Content-Type" content="text/html; charset=ISO-8859-1">`,
			`<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := string(normalizeHTML([]byte(tc.input)))
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNormalizeHTML_AttributeWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"double space in content attribute",
			`<meta name="keywords" content="UFO,  Burlington">`,
			`<meta name="keywords" content="UFO, Burlington">`,
		},
		{
			"multiple spaces collapsed",
			`<p title="a   b    c">text</p>`,
			`<p title="a b c">text</p>`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := string(normalizeHTML([]byte(tc.input)))
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNormalizeHTML_EntityDecoding(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"gt entity in text",
			`<p>foo &gt; bar</p>`,
			`<p>foo > bar</p>`,
		},
		{
			"copy entity in text",
			`<p>&copy; 2008</p>`,
			`<p>© 2008</p>`,
		},
		{
			"numeric entity",
			`<p>it&#39;s here</p>`,
			`<p>it's here</p>`,
		},
		{
			"entity in attribute value",
			`<a title="a &amp; b">link</a>`,
			`<a title="a & b">link</a>`,
		},
		{
			"already decoded stays same",
			`<p>© 2008</p>`,
			`<p>© 2008</p>`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := string(normalizeHTML([]byte(tc.input)))
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNormalizeHTML_InterTagWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"newline between tags collapsed",
			"<li>\n          <ul>",
			"<li><ul>",
		},
		{
			"whitespace between closing and opening",
			"</li>\n        <li>",
			"</li><li>",
		},
		{
			"text content preserved",
			"<p>hello world</p>",
			"<p>hello world</p>",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := string(normalizeHTML([]byte(tc.input)))
			assert.Equal(t, tc.expected, result)
		})
	}
}
