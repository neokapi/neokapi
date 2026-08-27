package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestScenariosStayDownstreamClean.
//
// This suite publishes its dataset into web/src, which the neokapi docs site
// serves and which scripts/check-docs-bowrain-clean.sh holds to zero mentions
// of the downstream platform. A scenario prompt or fixture path naming it puts
// that name into the committed JSON and fails a CI job three steps away from
// anything that looks like this file.
//
// It cost one red build. The rule is not arbitrary: the framework's docs
// describe the framework, and an eval about onboarding to the platform belongs
// in the platform's own suite.
func TestScenariosStayDownstreamClean(t *testing.T) {
	for _, sc := range scenarios {
		t.Run(sc.ID, func(t *testing.T) {
			assert.NotContains(t, strings.ToLower(sc.Prompt), "bowrain",
				"the prompt is copied verbatim into the published dataset")
			assert.NotContains(t, strings.ToLower(sc.Why), "bowrain")
			assert.NotContains(t, strings.ToLower(sc.Path), "bowrain")
			for _, f := range sc.Fixture {
				assert.NotContains(t, strings.ToLower(f.From), "bowrain",
					"a fixture path is recorded in the dataset, so its directory name ships too")
				assert.NotContains(t, strings.ToLower(f.Body), "bowrain")
				assert.NotContains(t, strings.ToLower(f.Note), "bowrain")
			}
		})
	}
}
