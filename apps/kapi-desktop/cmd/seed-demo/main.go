// Command seed-demo populates a Kapi Desktop config dir with a real, coherent
// sample terms + content memory, using the same framework packages the
// app reads (terms, memory). It honors KAPI_CONFIG_DIR (the desktop's
// config override) so it can target an isolated root:
//
//	KAPI_CONFIG_DIR=/tmp/iso/kapi go run -tags fts5 ./cmd/seed-demo
//
// Both the terms store and content-memory search order by updated_at DESC, so entries are
// written newest-first in the order the walkthrough narration expects.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

func hoursAgo(h int) time.Time { return time.Now().Add(-time.Duration(h) * time.Hour) }

func text(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }
func bOpen() model.Run {
	return model.Run{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>", Equiv: "b"}}
}
func bClose() model.Run {
	return model.Run{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>", Equiv: "b"}}
}
func person(name string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: "e1", Type: "entity:person", Data: name, Equiv: name}}
}

func main() {
	root := os.Getenv("KAPI_CONFIG_DIR")
	if root == "" {
		cfg, err := os.UserConfigDir()
		must(err)
		root = filepath.Join(cfg, "kapi")
	}
	tbDir := filepath.Join(root, "terms")
	memoryDir := filepath.Join(root, "memory")
	must(os.MkdirAll(tbDir, 0o755))
	must(os.MkdirAll(memoryDir, 0o755))

	seedTerms(filepath.Join(tbDir, "product-terms.db"))
	seedSecondaryTerms(filepath.Join(tbDir, "brand-terms.db"))
	seedMemory(filepath.Join(memoryDir, "acme-app.db"))
	seedSecondaryMemory(filepath.Join(memoryDir, "global-memory.db"))
	seedProviders(filepath.Join(root, "providers.json"))

	fmt.Println("seeded:", tbDir, memoryDir)
}

// seedProviders writes demo AI-provider configs (names + types only, no API
// keys) so the AI Credentials screen looks realistic without exposing the
// developer's real keychain entries. The backend reads this from the isolated
// KAPI_CONFIG_DIR (see backend NewApp), so the user's own providers.json is
// never touched.
func seedProviders(path string) {
	type provider struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		ProviderType string `json:"provider_type"`
		Model        string `json:"model,omitempty"`
	}
	providers := []provider{
		{ID: "p1", Name: "Claude (Anthropic)", ProviderType: "anthropic", Model: "claude-sonnet-4-6"},
		{ID: "p2", Name: "GPT-4o", ProviderType: "openai", Model: "gpt-4o"},
		{ID: "p3", Name: "Gemini", ProviderType: "gemini", Model: "gemini-2.5-flash"},
	}
	data, err := json.MarshalIndent(providers, "", "  ")
	must(err)
	must(os.WriteFile(path, data, 0o644))
}

func seedTerms(path string) {
	_ = os.Remove(path)
	tb, err := terms.NewSQLiteStore(path)
	must(err)
	// Display order (newest updated_at first). "seat" leads so the concept
	// spotlight lands on a card showing both approved and deprecated terms.
	concepts := []terms.Concept{
		{
			Domain: "Billing", Definition: "A paid licence assigned to one member of a workspace.",
			Terms: []terms.Term{
				{Text: "seat", Locale: "en", Status: model.TermPreferred},
				{Text: "siège", Locale: "fr", Status: model.TermApproved},
				{Text: "licence", Locale: "fr", Status: model.TermDeprecated, Note: "Use 'siège'."},
				{Text: "Sitzplatz", Locale: "de", Status: model.TermApproved},
			},
		},
		{
			Domain: "Product", Definition: "The landing screen summarising a workspace's key metrics.",
			Terms: []terms.Term{
				{Text: "dashboard", Locale: "en", Status: model.TermPreferred, PartOfSpeech: "noun"},
				{Text: "tableau de bord", Locale: "fr", Status: model.TermApproved},
				{Text: "Dashboard", Locale: "de", Status: model.TermApproved},
				{Text: "ダッシュボード", Locale: "ja", Status: model.TermApproved},
			},
		},
		{
			Domain: "Product", Definition: "A container that groups projects, members and billing.",
			Terms: []terms.Term{
				{Text: "workspace", Locale: "en", Status: model.TermPreferred},
				{Text: "espace de travail", Locale: "fr", Status: model.TermApproved},
				{Text: "Arbeitsbereich", Locale: "de", Status: model.TermApproved},
			},
		},
		{
			Domain: "Marketing", Definition: "The guided first-run experience for new members.",
			Terms: []terms.Term{
				{Text: "onboarding", Locale: "en", Status: model.TermPreferred},
				{Text: "intégration", Locale: "fr", Status: model.TermApproved},
				{Text: "Einarbeitung", Locale: "de", Status: model.TermProposed},
			},
		},
		{
			Domain: "Engineering", Definition: "An HTTP callback fired when an event occurs.",
			Terms: []terms.Term{
				{Text: "webhook", Locale: "en", Status: model.TermPreferred},
				{Text: "webhook", Locale: "fr", Status: model.TermApproved, Note: "Keep in English."},
				{Text: "Webhook", Locale: "de", Status: model.TermApproved},
			},
		},
		{
			Domain: "Analytics", Definition: "The share of users who return over a period.",
			Terms: []terms.Term{
				{Text: "retention", Locale: "en", Status: model.TermPreferred},
				{Text: "rétention", Locale: "fr", Status: model.TermApproved},
				{Text: "Bindung", Locale: "de", Status: model.TermApproved},
			},
		},
		{
			Domain: "Billing", Definition: "A document itemising charges for a billing period.",
			Terms: []terms.Term{
				{Text: "invoice", Locale: "en", Status: model.TermPreferred},
				{Text: "facture", Locale: "fr", Status: model.TermApproved},
				{Text: "Rechnung", Locale: "de", Status: model.TermApproved},
			},
		},
		{
			Domain: "Product", Definition: "Downloading data out of the app in a portable format.",
			Terms: []terms.Term{
				{Text: "export", Locale: "en", Status: model.TermPreferred},
				{Text: "exporter", Locale: "fr", Status: model.TermApproved, PartOfSpeech: "verb"},
				{Text: "Export", Locale: "de", Status: model.TermApproved},
			},
		},
	}
	for i, c := range concepts {
		c.ID = fmt.Sprintf("c-%02d", i+1)
		c.Source = terms.TermSourceTerminology
		// Spread created_at across ~2 weeks so the Activity chart has points;
		// updated_at stays near-now (small offsets) to drive display order.
		c.CreatedAt = hoursAgo(i*42 + 18)
		c.UpdatedAt = hoursAgo(i * 3) // i=0 newest → first
		must(tb.AddConcept(context.Background(), c))
	}
}

func seedSecondaryTerms(path string) {
	_ = os.Remove(path)
	tb, err := terms.NewSQLiteStore(path)
	must(err)
	concepts := []terms.Concept{
		{Domain: "Brand", Definition: "The product name, never translated.", Terms: []terms.Term{
			{Text: "Acme", Locale: "en", Status: model.TermPreferred},
			{Text: "Acme", Locale: "fr", Status: model.TermForbidden, Note: "Do not translate."},
		}},
		{Domain: "Brand", Definition: "The tone we use with customers.", Terms: []terms.Term{
			{Text: "friendly", Locale: "en", Status: model.TermPreferred},
		}},
	}
	for i, c := range concepts {
		c.ID = fmt.Sprintf("b-%02d", i+1)
		c.Source = terms.TermSourceTerminology
		c.CreatedAt = hoursAgo(72)
		c.UpdatedAt = hoursAgo(48)
		must(tb.AddConcept(context.Background(), c))
	}
}

func seedMemory(path string) {
	_ = os.Remove(path)
	tm, err := memory.NewSQLiteStore(path)
	must(err)
	// Display order (newest updated_at first): a simple multilingual string,
	// then one carrying inline bold, then the entity-bearing one, then the
	// "invite" string the memory search lands on.
	variants := []map[model.LocaleID][]model.Run{
		{
			"en": {text("Welcome back")},
			"fr": {text("Bon retour")},
			"de": {text("Willkommen zurück")},
		},
		{
			"en": {text("Click "), bOpen(), text("here"), bClose(), text(" to continue")},
			"fr": {text("Cliquez "), bOpen(), text("ici"), bClose(), text(" pour continuer")},
		},
		{
			"en": {text("Hi "), person("Bob"), text(", your report is ready")},
			"fr": {text("Bonjour "), person("Bob"), text(", votre rapport est prêt")},
		},
		{
			"en": {text("Invite teammates to your workspace")},
			"fr": {text("Invitez des collègues dans votre espace de travail")},
			"de": {text("Laden Sie Teammitglieder in Ihren Arbeitsbereich ein")},
		},
		{
			"en": {text("Your invoice is ready")},
			"fr": {text("Votre facture est prête")},
		},
		{
			"en": {text("Settings saved")},
			"fr": {text("Paramètres enregistrés")},
			"de": {text("Einstellungen gespeichert")},
		},
		{
			"en": {text("Export your data")},
			"fr": {text("Exportez vos données")},
		},
	}
	for i, v := range variants {
		must(tm.Add(context.Background(), memory.Entry{
			ID:          fmt.Sprintf("tm-%02d", i+1),
			Variants:    v,
			HintSrcLang: "en",
			// Spread created_at across ~2 weeks for the Activity chart; updated_at
			// stays near-now to drive display order.
			CreatedAt: hoursAgo(i*50 + 18),
			UpdatedAt: hoursAgo(i * 6), // i=0 newest → first
		}))
	}
}

func seedSecondaryMemory(path string) {
	_ = os.Remove(path)
	tm, err := memory.NewSQLiteStore(path)
	must(err)
	must(tm.Add(context.Background(), memory.Entry{
		ID:          "g-01",
		Variants:    map[model.LocaleID][]model.Run{"en": {text("Save changes")}, "fr": {text("Enregistrer les modifications")}},
		HintSrcLang: "en",
		CreatedAt:   hoursAgo(72),
		UpdatedAt:   hoursAgo(72),
	}))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed error:", err)
		os.Exit(1)
	}
}
