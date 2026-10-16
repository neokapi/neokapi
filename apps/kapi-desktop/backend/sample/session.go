package sample

import (
	"context"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/host"
)

// The sample arrives with an assistant's notes to review.
//
// Two sessions are recorded through the host API, as an assistant records
// them over MCP: claude-code, the earlier one, notices two names, a word and
// how the store addresses the shopper, and records a correction a person made;
// codex, the later one, takes the other side of that correction and notices
// one more name. Every piece of evidence quotes a sentence in the sample's own
// files, so a person opening the file finds it there.
//
// What's new then opens with one disagreement to settle, three names and
// spellings, one word and one note on how the project writes: the review loop
// on the real surfaces, with no AI call.

// The sessions' ids. Stable, so History can name them and a reset to before
// one finds it on every machine.
const (
	ClaudeCodeSession = "kapimart-sample-claude-code"
	CodexSession      = "kapimart-sample-codex"
)

// SeedSessions records the sample's assistant sessions in the project at
// recipe, unless its log already holds an agent session: then a person has
// seen them, or an assistant has worked here since, and nothing is added. It
// reports whether it recorded them. A sample with no sessions of its own
// records nothing.
func SeedSessions(ctx context.Context, app *host.App, name, recipe string) (bool, error) {
	if name != "kapimart" {
		return false, nil
	}
	log, err := app.ContextOperations(ctx, host.ContextLogRequest{Project: recipe})
	if err != nil {
		return false, fmt.Errorf("read the sample's context history: %w", err)
	}
	for _, op := range log.Operations {
		if op.Actor.Kind == contextop.ActorAgent {
			return false, nil
		}
	}

	// The team's notes came first; the assistants worked here after them.
	if err := seedTeamNotes(ctx, app, recipe); err != nil {
		return false, err
	}

	claude := contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: ClaudeCodeSession}
	codex := contextop.Actor{Kind: contextop.ActorAgent, Name: "codex", Session: CodexSession}
	seen := func(path, quote string) []contextop.Evidence {
		return []contextop.Evidence{{Path: path, Quote: quote}}
	}
	observations := []host.ContextObserveRequest{
		{
			Actor: claude, Term: "KapiMart", InsteadOf: []string{"Kapi Mart"},
			Text:     "The product name is written as one word.",
			Evidence: seen("web/en/getting-started.md", "Welcome to the KapiMart Developer Platform!"),
		},
		{
			Actor: claude, Term: "cart", InsteadOf: []string{"basket"},
			Text:     "The store calls it a cart.",
			Evidence: seen("src/en/store-ui.json", "Your cart is empty"),
		},
		{
			Actor: claude, Term: "checkout", InsteadOf: []string{"check-out"},
			Text:     "Checkout is one word, as a noun and as an adjective.",
			Evidence: seen("web/en/faq.md", "offer localized checkout experiences"),
		},
	}
	for _, req := range observations {
		req.Project = recipe
		if _, err := app.RecordContextObservation(ctx, req); err != nil {
			return false, fmt.Errorf("record the sample's session: %w", err)
		}
	}
	if _, err := app.RecordContextCorrection(ctx, host.ContextCorrectRequest{
		Actor: claude, Project: recipe,
		From: "log in", To: "sign in", Suggest: true,
		Note:     "A person changed this in the error messages.",
		Evidence: seen("src/en/error-messages.properties", "Your session has expired. Please sign in again to continue."),
	}); err != nil {
		return false, fmt.Errorf("record the sample's session: %w", err)
	}
	if _, err := app.RecordContextObservation(ctx, host.ContextObserveRequest{
		Actor: claude, Project: recipe,
		Text:     "The store copy addresses the shopper as you.",
		Evidence: seen("src/en/store-ui.json", "Thank you! Your order has been confirmed."),
	}); err != nil {
		return false, fmt.Errorf("record the sample's session: %w", err)
	}

	later := []host.ContextObserveRequest{
		{
			Actor: codex, Term: "log in", InsteadOf: []string{"sign in"},
			Text:     "The other storefronts say log in.",
			Evidence: seen("src/en/error-messages.properties", "Please verify your email address before signing in."),
		},
		{
			Actor: codex, Term: "PayPal", InsteadOf: []string{"Paypal"},
			Text:     "The payment provider's name keeps its capital P.",
			Evidence: seen("src/en/checkout-flow.json", "You will be redirected to PayPal to complete your purchase."),
		},
	}
	for _, req := range later {
		req.Project = recipe
		if _, err := app.RecordContextObservation(ctx, req); err != nil {
			return false, fmt.Errorf("record the sample's session: %w", err)
		}
	}
	return true, nil
}

// TeamMember is the person whose standing notes the sample arrives with.
const TeamMember = "maria.lund@kapimart.example"

// seedTeamNotes records what a person on the team wrote down and kept: notes
// that hold in one market, for one reader age, or for a while only. They are
// established from the start, so What's new still opens on the assistants'
// suggestions, and Explore has a market, an age range and lifetimes to draw.
// The windows are set from today, so whenever the sample is opened one is in
// force, one has ended and one has yet to begin.
func seedTeamNotes(ctx context.Context, app *host.App, recipe string) error {
	team := contextop.Actor{Kind: contextop.ActorPerson, Name: TeamMember}
	day := func(offset int) string { return time.Now().UTC().AddDate(0, 0, offset).Format(time.DateOnly) }
	seen := func(quote string) []contextop.Evidence {
		return []contextop.Evidence{{Path: "src/en/store-ui.json", Quote: quote}}
	}
	notes := []host.ContextObserveRequest{
		{
			Point:    map[string]string{"market": "jp"},
			Text:     "Prices in Japan include consumption tax, so the tax line reads as included.",
			Evidence: seen("Estimated Tax"),
		},
		{
			Point:    map[string]string{"age": "13-17"},
			Text:     "For readers aged 13 to 17 the shipping notice states the threshold without an exclamation mark.",
			Evidence: seen("Free shipping on orders over {amount}!"),
		},
		{
			Point: map[string]string{"market": "nordics"}, ValidFrom: day(-10), ValidTo: day(30),
			Text:     "During the winter campaign the Nordic store names the campaign in the shipping notice.",
			Evidence: seen("Free shipping on orders over {amount}!"),
		},
		{
			ValidFrom: day(-40), ValidTo: day(-5),
			Text:     "The autumn sale labelled reduced prices Sale Price; that label ended with the sale.",
			Evidence: seen("Sale Price"),
		},
		{
			Point: map[string]string{"market": "de"}, ValidFrom: day(20),
			Text:     "Overnight shipping to Germany starts next month and is called Express Plus there.",
			Evidence: seen("Overnight Shipping (next business day)"),
		},
	}
	for _, req := range notes {
		req.Actor, req.Project = team, recipe
		noted, err := app.RecordContextObservation(ctx, req)
		if err != nil {
			return fmt.Errorf("record the sample's team notes: %w", err)
		}
		if _, err := app.KeepContextOperation(ctx, host.ContextKeepRequest{
			Actor: team, Project: recipe, ID: noted.ID,
		}); err != nil {
			return fmt.Errorf("keep the sample's team notes: %w", err)
		}
	}
	return nil
}
