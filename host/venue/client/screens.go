package client

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/neokapi/neokapi/host/screens"
)

// The project screens a Bowrain server draws: the Map, the Reader, the
// questions Decide asks and the rules in force, each at the client's stream.
// The server answers each with the host/screens DTO the desktop draws the
// same screen from.

// screenPath is the path of one screen route under the server base URL,
// /api/v1/<ws>/<project>/<route>.
func (c *BowrainClient) screenPath(route string) string {
	return strings.TrimPrefix(c.projectPrefix(), c.baseURL) + "/" + route
}

// MapSummary reads the project's Map: GET /:ws/:id/map/:ref.
func (c *BowrainClient) MapSummary(ctx context.Context) (*screens.MapSummaryDTO, error) {
	var out screens.MapSummaryDTO
	if err := c.DoJSON(ctx, http.MethodGet, c.screenPath("map/"+c.ref()), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReaderFile reads one item of the project in the Reader:
// GET /:ws/:id/reader/:ref?item=<name>.
func (c *BowrainClient) ReaderFile(ctx context.Context, item string) (*screens.ReaderFileDTO, error) {
	var out screens.ReaderFileDTO
	q := url.Values{"item": {item}}
	if err := c.DoJSON(ctx, http.MethodGet, c.screenPath("reader/"+c.ref()), q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RulesDecideView reads the questions waiting for a decision:
// GET /:ws/:id/rules/:ref/decide.
func (c *BowrainClient) RulesDecideView(ctx context.Context) (*screens.RulesDecideViewDTO, error) {
	var out screens.RulesDecideViewDTO
	if err := c.DoJSON(ctx, http.MethodGet, c.screenPath("rules/"+c.ref()+"/decide"), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// KeptRules reads the rules in force and who kept each:
// GET /:ws/:id/rules/:ref/kept.
func (c *BowrainClient) KeptRules(ctx context.Context) ([]screens.KeptRuleDTO, error) {
	out := []screens.KeptRuleDTO{}
	if err := c.DoJSON(ctx, http.MethodGet, c.screenPath("rules/"+c.ref()+"/kept"), nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
