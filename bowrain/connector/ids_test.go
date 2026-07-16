package connector

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestURLSafeID(t *testing.T) {
	cases := map[string]string{
		"https://demo.wp-api.org":      "https-demo.wp-api.org",
		"https://blog.example.com/sub": "https-blog.example.com-sub",
		"Plain_Name-1.2":               "plain_name-1.2",
		"::weird///":                   "weird",
	}
	for in, want := range cases {
		assert.Equal(t, want, urlSafeID(in), in)
	}
}
