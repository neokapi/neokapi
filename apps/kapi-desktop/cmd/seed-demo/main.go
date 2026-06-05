// Command seed-demo populates a Kapi Desktop config dir with a real, coherent
// sample termbase + translation memory, using the same framework packages the
// app reads (termbase, sievepen). It honors KAPI_CONFIG_DIR (the desktop's
// config override) so it can target an isolated root:
//
//	KAPI_CONFIG_DIR=/tmp/iso/kapi go run -tags fts5 ./cmd/seed-demo
//
// Both the termbase and TM search order by updated_at DESC, so entries are
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
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
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
	tbDir := filepath.Join(root, "termbases")
	tmDir := filepath.Join(root, "tm")
	must(os.MkdirAll(tbDir, 0o755))
	must(os.MkdirAll(tmDir, 0o755))

	seedTermbase(filepath.Join(tbDir, "product-glossary.db"))
	seedSecondaryTermbase(filepath.Join(tbDir, "brand-terms.db"))
	seedTM(filepath.Join(tmDir, "acme-app.db"))
	seedSecondaryTM(filepath.Join(tmDir, "global-tm.db"))
	seedProviders(filepath.Join(root, "providers.json"))

	fmt.Println("seeded:", tbDir, tmDir)
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

func seedTermbase(path string) {
	_ = os.Remove(path)
	tb, err := termbase.NewSQLiteTermBase(path)
	must(err)
	// Display order (newest updated_at first). "seat" leads so the concept
	// spotlight lands on a card showing both approved and deprecated terms.
	concepts := []termbase.Concept{
		{
			Domain: "Billing", Definition: "A paid licence assigned to one member of a workspace.",
			Terms: []termbase.Term{
				{Text: "seat", Locale: "en-US", Status: model.TermPreferred},
				{Text: "siège", Locale: "fr-FR", Status: model.TermApproved},
				{Text: "licence", Locale: "fr-FR", Status: model.TermDeprecated, Note: "Use 'siège'."},
				{Text: "Sitzplatz", Locale: "de-DE", Status: model.TermApproved},
			},
		},
		{
			Domain: "Product", Definition: "The landing screen summarising a workspace's key metrics.",
			Terms: []termbase.Term{
				{Text: "dashboard", Locale: "en-US", Status: model.TermPreferred, PartOfSpeech: "noun"},
				{Text: "tableau de bord", Locale: "fr-FR", Status: model.TermApproved},
				{Text: "Dashboard", Locale: "de-DE", Status: model.TermApproved},
				{Text: "ダッシュボード", Locale: "ja-JP", Status: model.TermApproved},
			},
		},
		{
			Domain: "Product", Definition: "A container that groups projects, members and billing.",
			Terms: []termbase.Term{
				{Text: "workspace", Locale: "en-US", Status: model.TermPreferred},
				{Text: "espace de travail", Locale: "fr-FR", Status: model.TermApproved},
				{Text: "Arbeitsbereich", Locale: "de-DE", Status: model.TermApproved},
			},
		},
		{
			Domain: "Marketing", Definition: "The guided first-run experience for new members.",
			Terms: []termbase.Term{
				{Text: "onboarding", Locale: "en-US", Status: model.TermPreferred},
				{Text: "intégration", Locale: "fr-FR", Status: model.TermApproved},
				{Text: "Einarbeitung", Locale: "de-DE", Status: model.TermProposed},
			},
		},
		{
			Domain: "Engineering", Definition: "An HTTP callback fired when an event occurs.",
			Terms: []termbase.Term{
				{Text: "webhook", Locale: "en-US", Status: model.TermPreferred},
				{Text: "webhook", Locale: "fr-FR", Status: model.TermApproved, Note: "Keep in English."},
				{Text: "Webhook", Locale: "de-DE", Status: model.TermApproved},
			},
		},
		{
			Domain: "Analytics", Definition: "The share of users who return over a period.",
			Terms: []termbase.Term{
				{Text: "retention", Locale: "en-US", Status: model.TermPreferred},
				{Text: "rétention", Locale: "fr-FR", Status: model.TermApproved},
				{Text: "Bindung", Locale: "de-DE", Status: model.TermApproved},
			},
		},
		{
			Domain: "Billing", Definition: "A document itemising charges for a billing period.",
			Terms: []termbase.Term{
				{Text: "invoice", Locale: "en-US", Status: model.TermPreferred},
				{Text: "facture", Locale: "fr-FR", Status: model.TermApproved},
				{Text: "Rechnung", Locale: "de-DE", Status: model.TermApproved},
			},
		},
		{
			Domain: "Product", Definition: "Downloading data out of the app in a portable format.",
			Terms: []termbase.Term{
				{Text: "export", Locale: "en-US", Status: model.TermPreferred},
				{Text: "exporter", Locale: "fr-FR", Status: model.TermApproved, PartOfSpeech: "verb"},
				{Text: "Export", Locale: "de-DE", Status: model.TermApproved},
			},
		},
	}
	for i, c := range concepts {
		c.ID = fmt.Sprintf("c-%02d", i+1)
		c.Source = termbase.TermSourceTerminology
		// Spread created_at across ~2 weeks so the Activity chart has points;
		// updated_at stays near-now (small offsets) to drive display order.
		c.CreatedAt = hoursAgo(i*42 + 18)
		c.UpdatedAt = hoursAgo(i * 3) // i=0 newest → first
		must(tb.AddConcept(context.Background(), c))
	}
}

func seedSecondaryTermbase(path string) {
	_ = os.Remove(path)
	tb, err := termbase.NewSQLiteTermBase(path)
	must(err)
	concepts := []termbase.Concept{
		{Domain: "Brand", Definition: "The product name — never translated.", Terms: []termbase.Term{
			{Text: "Acme", Locale: "en-US", Status: model.TermPreferred},
			{Text: "Acme", Locale: "fr-FR", Status: model.TermForbidden, Note: "Do not translate."},
		}},
		{Domain: "Brand", Definition: "The tone we use with customers.", Terms: []termbase.Term{
			{Text: "friendly", Locale: "en-US", Status: model.TermPreferred},
		}},
	}
	for i, c := range concepts {
		c.ID = fmt.Sprintf("b-%02d", i+1)
		c.Source = termbase.TermSourceTerminology
		c.CreatedAt = hoursAgo(72)
		c.UpdatedAt = hoursAgo(48)
		must(tb.AddConcept(context.Background(), c))
	}
}

func seedTM(path string) {
	_ = os.Remove(path)
	tm, err := sievepen.NewSQLiteTM(path)
	must(err)
	// Display order (newest updated_at first): a simple multilingual string,
	// then one carrying inline bold, then the entity-bearing one, then the
	// "invite" string the memory search lands on.
	variants := []map[model.LocaleID][]model.Run{
		{
			"en-US": {text("Welcome back")},
			"fr-FR": {text("Bon retour")},
			"de-DE": {text("Willkommen zurück")},
		},
		{
			"en-US": {text("Click "), bOpen(), text("here"), bClose(), text(" to continue")},
			"fr-FR": {text("Cliquez "), bOpen(), text("ici"), bClose(), text(" pour continuer")},
		},
		{
			"en-US": {text("Hi "), person("Bob"), text(", your report is ready")},
			"fr-FR": {text("Bonjour "), person("Bob"), text(", votre rapport est prêt")},
		},
		{
			"en-US": {text("Invite teammates to your workspace")},
			"fr-FR": {text("Invitez des collègues dans votre espace de travail")},
			"de-DE": {text("Laden Sie Teammitglieder in Ihren Arbeitsbereich ein")},
		},
		{
			"en-US": {text("Your invoice is ready")},
			"fr-FR": {text("Votre facture est prête")},
		},
		{
			"en-US": {text("Settings saved")},
			"fr-FR": {text("Paramètres enregistrés")},
			"de-DE": {text("Einstellungen gespeichert")},
		},
		{
			"en-US": {text("Export your data")},
			"fr-FR": {text("Exportez vos données")},
		},
	}
	for i, v := range variants {
		must(tm.Add(context.Background(), sievepen.TMEntry{
			ID:          fmt.Sprintf("tm-%02d", i+1),
			Variants:    v,
			HintSrcLang: "en-US",
			// Spread created_at across ~2 weeks for the Activity chart; updated_at
			// stays near-now to drive display order.
			CreatedAt: hoursAgo(i*50 + 18),
			UpdatedAt: hoursAgo(i * 6), // i=0 newest → first
		}))
	}
}

func seedSecondaryTM(path string) {
	_ = os.Remove(path)
	tm, err := sievepen.NewSQLiteTM(path)
	must(err)
	must(tm.Add(context.Background(), sievepen.TMEntry{
		ID:          "g-01",
		Variants:    map[model.LocaleID][]model.Run{"en-US": {text("Save changes")}, "fr-FR": {text("Enregistrer les modifications")}},
		HintSrcLang: "en-US",
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
