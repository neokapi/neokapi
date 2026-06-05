// Package sample provides embedded sample projects for the kapi-desktop app.
// Two sample projects ("kapimart" and "okapimart") share identical source files
// but use different format engines — native Go vs Okapi Bridge — so users can
// compare them side by side.
package sample

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
)

//go:embed shared/* kapimart/* okapimart/*
var assetsFS embed.FS

// DisplayName maps an internal sample name to its user-facing name.
var DisplayName = map[string]string{
	"kapimart":  "KapiMart",
	"okapimart": "OkapiMart",
}

// List returns the available sample project names.
func List() []string {
	return []string{"kapimart", "okapimart"}
}

// Scaffold creates a sample project on disk at targetDir.
// name must be "kapimart" or "okapimart".
func Scaffold(name, targetDir string) error {
	if _, ok := DisplayName[name]; !ok {
		return fmt.Errorf("unknown sample project %q", name)
	}

	// Copy input files — kapimart v2 has its own content, okapimart uses shared.
	inputSrc := "shared/input"
	if name == "kapimart" {
		inputSrc = "kapimart/input"
	}
	if err := copyEmbeddedDir(inputSrc, filepath.Join(targetDir, "input")); err != nil {
		return fmt.Errorf("copy input files: %w", err)
	}

	// Copy the project-specific .kapi file.
	kapiData, err := assetsFS.ReadFile(name + "/project.kapi")
	if err != nil {
		return fmt.Errorf("read project.kapi: %w", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create project dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "project.kapi"), kapiData, 0o644); err != nil {
		return fmt.Errorf("write project.kapi: %w", err)
	}

	// Create output directory.
	if err := os.MkdirAll(filepath.Join(targetDir, "output"), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	kapiDir := filepath.Join(targetDir, ".kapi")
	if err := os.MkdirAll(kapiDir, 0o755); err != nil {
		return fmt.Errorf("create .kapi dir: %w", err)
	}

	// Seed TM and termbase — v2 for kapimart, v1 for okapimart.
	if name == "kapimart" {
		if err := seedTMv2(filepath.Join(kapiDir, "tm.db")); err != nil {
			return fmt.Errorf("seed TM: %w", err)
		}
		if err := seedTermbasev2(filepath.Join(kapiDir, "termbase.db")); err != nil {
			return fmt.Errorf("seed termbase: %w", err)
		}
	} else {
		if err := seedTM(filepath.Join(kapiDir, "tm.db")); err != nil {
			return fmt.Errorf("seed TM: %w", err)
		}
		if err := seedTermbase(filepath.Join(kapiDir, "termbase.db")); err != nil {
			return fmt.Errorf("seed termbase: %w", err)
		}
	}

	return nil
}

// --- OkapiMart v1 seed functions (unchanged) ---

func seedTM(dbPath string) error {
	tmxData, err := assetsFS.ReadFile("shared/tm-seed.tmx")
	if err != nil {
		return fmt.Errorf("read TMX: %w", err)
	}
	tm, err := sievepen.NewSQLiteTM(dbPath)
	if err != nil {
		return err
	}
	defer tm.Close()
	// The TMX already has all target locales on each TU; a single import
	// creates one multilingual entry per TU with every variant populated.
	if _, _, err := sievepen.ImportTMXSession(context.Background(), tm, bytes.NewReader(tmxData),
		sievepen.ImportTMXOptions{
			OriginKey:     "tm-seed.tmx",
			OriginAddedBy: "kapi-sample",
		}); err != nil {
		return fmt.Errorf("import TMX: %w", err)
	}
	spreadTimestamps(tm.DB(), "tm_entries", 30)
	return nil
}

func seedTermbase(dbPath string) error {
	tbData, err := assetsFS.ReadFile("shared/termbase-seed.json")
	if err != nil {
		return fmt.Errorf("read termbase JSON: %w", err)
	}
	tb, err := termbase.NewSQLiteTermBase(dbPath)
	if err != nil {
		return err
	}
	defer tb.Close()
	if _, err := termbase.ImportJSON(context.Background(), tb, bytes.NewReader(tbData)); err != nil {
		return fmt.Errorf("import termbase: %w", err)
	}
	spreadTimestamps(tb.DB(), "tb_concepts", 30)
	return nil
}

// --- KapiMart v2 seed functions ---

var v2Targets = []model.LocaleID{"de-DE", "fr-FR", "ja-JP", "nb-NO", "ar-SA"}

func seedTMv2(dbPath string) error {
	tmxData, err := assetsFS.ReadFile("kapimart/tm-seed.tmx")
	if err != nil {
		return fmt.Errorf("read TMX: %w", err)
	}
	tm, err := sievepen.NewSQLiteTM(dbPath)
	if err != nil {
		return err
	}
	defer tm.Close()

	// The TMX already has all target locales on each TU; a single import
	// creates one multilingual entry per TU with every variant populated.
	if _, _, err := sievepen.ImportTMXSession(context.Background(), tm, bytes.NewReader(tmxData),
		sievepen.ImportTMXOptions{
			OriginKey:     "tm-seed.tmx",
			OriginAddedBy: "kapi-sample",
		}); err != nil {
		return fmt.Errorf("import TMX: %w", err)
	}

	// Add enriched entries with structural inline codes and entity annotations.
	if err := seedEnrichedEntries(tm); err != nil {
		return fmt.Errorf("seed enriched entries: %w", err)
	}

	// Spread timestamps over 90 days for a realistic activity chart.
	spreadTimestamps(tm.DB(), "tm_entries", 90)
	return nil
}

func seedTermbasev2(dbPath string) error {
	tbData, err := assetsFS.ReadFile("kapimart/termbase-seed.json")
	if err != nil {
		return fmt.Errorf("read termbase JSON: %w", err)
	}
	tb, err := termbase.NewSQLiteTermBase(dbPath)
	if err != nil {
		return err
	}
	defer tb.Close()
	if _, err := termbase.ImportJSON(context.Background(), tb, bytes.NewReader(tbData)); err != nil {
		return fmt.Errorf("import termbase: %w", err)
	}
	spreadTimestamps(tb.DB(), "tb_concepts", 90)
	return nil
}

// --- Enriched TM entries (structural + entity) ---

// enrichedEntry defines a TM entry with inline codes and/or entity placeholders.
// The source is always in en-US; targets maps each supported locale to a
// Run-sequence factory. Entities, when set, carry the placeholder ID, type,
// and the en-US value; per-locale entity values are not defined separately
// for sample data.
type enrichedEntry struct {
	source   func() []model.Run
	targets  map[model.LocaleID]func() []model.Run
	entities []enrichedEntity
}

// enrichedEntity is the sample-file shape for an entity mapping — just
// the placeholder ID, type, and the en-US value. At seed time we expand
// this into a sievepen.EntityMapping with a Values map keyed by en-US.
type enrichedEntity struct {
	PlaceholderID string
	Type          model.EntityType
	SourceValue   string
}

// seedEnrichedEntries adds multilingual TM entries with structural markup
// and entity annotations that exercise all 6 match tiers. Each definition
// produces exactly one entry with en-US as the canonical source and all
// v2Targets as peer variants.
func seedEnrichedEntries(tm *sievepen.SQLiteTM) error {
	entries := enrichedEntryDefs()
	now := time.Now()
	for i, def := range entries {
		variants := map[model.LocaleID][]model.Run{
			"en-US": def.source(),
		}
		for _, tgt := range v2Targets {
			if fn, ok := def.targets[tgt]; ok {
				variants[tgt] = fn()
			}
		}
		entity := make([]sievepen.EntityMapping, 0, len(def.entities))
		for _, e := range def.entities {
			entity = append(entity, sievepen.EntityMapping{
				PlaceholderID: e.PlaceholderID,
				Type:          e.Type,
				Values: map[model.LocaleID]sievepen.EntityValue{
					"en-US": {Text: e.SourceValue},
				},
			})
		}
		entry := sievepen.TMEntry{
			ID:          id.New(),
			Variants:    variants,
			HintSrcLang: "en-US",
			Entities:    entity,
			Origins: []sievepen.Origin{
				{
					Source:    "import",
					Key:       fmt.Sprintf("sample/kapimart/enriched/%d", i),
					Reference: "seed",
					AddedAt:   now,
					AddedBy:   "kapi-sample",
				},
			},
		}
		if err := tm.Add(context.Background(), entry); err != nil {
			return fmt.Errorf("add enriched entry: %w", err)
		}
	}
	return nil
}

// textRun returns a TextRun unless the input is empty, in which case it
// returns nil so the caller can omit the slot.
func textRun(s string) (model.Run, bool) {
	if s == "" {
		return model.Run{}, false
	}
	return model.Run{Text: &model.TextRun{Text: s}}, true
}

func appendText(runs []model.Run, s string) []model.Run {
	if r, ok := textRun(s); ok {
		return append(runs, r)
	}
	return runs
}

// Helper: bold-wrapped text as a Run sequence.
func boldRuns(before, bold, after string) []model.Run {
	runs := appendText(nil, before)
	runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}})
	runs = appendText(runs, bold)
	runs = append(runs, model.Run{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}})
	return appendText(runs, after)
}

// Helper: link-wrapped text as a Run sequence.
func linkRuns(before, linkText, after string) []model.Run {
	runs := appendText(nil, before)
	runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: "<a>"}})
	runs = appendText(runs, linkText)
	runs = append(runs, model.Run{PcClose: &model.PcCloseRun{ID: "1", Type: "link:hyperlink", Data: "</a>"}})
	return appendText(runs, after)
}

// Helper: entity placeholder as a Run sequence.
func entityRuns(before, entityType, entityValue, after string) []model.Run {
	runs := appendText(nil, before)
	runs = append(runs, model.Run{Ph: &model.PlaceholderRun{ID: "1", Type: "entity:" + entityType, Data: entityValue}})
	return appendText(runs, after)
}

// Helper: bold + entity as a Run sequence.
func boldEntityRuns(before, bold, mid, entityType, entityValue, after string) []model.Run {
	runs := appendText(nil, before)
	runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}})
	runs = appendText(runs, bold)
	runs = append(runs, model.Run{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}})
	runs = appendText(runs, mid)
	runs = append(runs, model.Run{Ph: &model.PlaceholderRun{ID: "2", Type: "entity:" + entityType, Data: entityValue}})
	return appendText(runs, after)
}

// Helper: bold Run-sequence factory.
func boldF(before, bold, after string) func() []model.Run {
	return func() []model.Run { return boldRuns(before, bold, after) }
}

// Helper: link Run-sequence factory.
func linkF(before, link, after string) func() []model.Run {
	return func() []model.Run { return linkRuns(before, link, after) }
}

// Helper: entity Run-sequence factory.
func entityF(before, eType, eVal, after string) func() []model.Run {
	return func() []model.Run { return entityRuns(before, eType, eVal, after) }
}

// Helper: bold+entity Run-sequence factory.
func boldEntityF(before, bold, mid, eType, eVal, after string) func() []model.Run {
	return func() []model.Run { return boldEntityRuns(before, bold, mid, eType, eVal, after) }
}

func enrichedEntryDefs() []enrichedEntry {
	return []enrichedEntry{
		// --- Structural entries (bold) ---
		{
			source: boldF("Click ", "here", " to view your order."),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldF("Klicken Sie ", "hier", ", um Ihre Bestellung anzuzeigen."),
				"fr-FR": boldF("Cliquez ", "ici", " pour voir votre commande."),
				"ja-JP": boldF("注文を表示するには", "こちら", "をクリックしてください。"),
				"nb-NO": boldF("Klikk ", "her", " for å se bestillingen din."),
				"ar-SA": boldF("انقر ", "هنا", " لعرض طلبك."),
			},
		},
		{
			source: boldF("Your ", "payment", " has been processed successfully."),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldF("Ihre ", "Zahlung", " wurde erfolgreich verarbeitet."),
				"fr-FR": boldF("Votre ", "paiement", " a été traité avec succès."),
				"ja-JP": boldF("お", "支払い", "は正常に処理されました。"),
				"nb-NO": boldF("Din ", "betaling", " er behandlet."),
				"ar-SA": boldF("تمت معالجة ", "الدفع", " بنجاح."),
			},
		},
		{
			source: boldF("Free shipping", "", " on all orders over $50!"),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldF("Kostenloser Versand", "", " für alle Bestellungen über 50 $!"),
				"fr-FR": boldF("Livraison gratuite", "", " pour toutes les commandes de plus de 50 $ !"),
				"ja-JP": boldF("送料無料", "", " — 50ドル以上のご注文が対象です！"),
				"nb-NO": boldF("Gratis frakt", "", " på alle bestillinger over 50 $!"),
				"ar-SA": boldF("شحن مجاني", "", " على جميع الطلبات التي تزيد عن 50 دولار!"),
			},
		},
		{
			source: boldF("Important:", " ", "Your account will be deactivated in 30 days."),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldF("Wichtig:", " ", "Ihr Konto wird in 30 Tagen deaktiviert."),
				"fr-FR": boldF("Important :", " ", "Votre compte sera désactivé dans 30 jours."),
				"ja-JP": boldF("重要：", "", "アカウントは30日後に無効になります。"),
				"nb-NO": boldF("Viktig:", " ", "Kontoen din deaktiveres om 30 dager."),
				"ar-SA": boldF("مهم:", " ", "سيتم إلغاء تفعيل حسابك خلال 30 يومًا."),
			},
		},
		{
			source: boldF("New!", " ", "Check out our summer collection."),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldF("Neu!", " ", "Entdecken Sie unsere Sommerkollektion."),
				"fr-FR": boldF("Nouveau !", " ", "Découvrez notre collection d'été."),
				"ja-JP": boldF("新着！", "", "サマーコレクションをご覧ください。"),
				"nb-NO": boldF("Nytt!", " ", "Sjekk ut sommersamlingen vår."),
				"ar-SA": boldF("جديد!", " ", "اطلع على مجموعة الصيف."),
			},
		},
		{
			source: boldF("Save 20%", "", " when you subscribe to our newsletter."),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldF("Sparen Sie 20 %", "", ", wenn Sie unseren Newsletter abonnieren."),
				"fr-FR": boldF("Économisez 20 %", "", " en vous abonnant à notre newsletter."),
				"ja-JP": boldF("20% 割引", "", " — ニュースレターに登録するとお得です。"),
				"nb-NO": boldF("Spar 20 %", "", " når du abonnerer på nyhetsbrevet vårt."),
				"ar-SA": boldF("وفر 20%", "", " عند الاشتراك في النشرة الإخبارية."),
			},
		},
		{
			source: boldF("Warning:", " ", "This action cannot be undone."),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldF("Warnung:", " ", "Diese Aktion kann nicht rückgängig gemacht werden."),
				"fr-FR": boldF("Attention :", " ", "Cette action est irréversible."),
				"ja-JP": boldF("警告：", "", "この操作は元に戻せません。"),
				"nb-NO": boldF("Advarsel:", " ", "Denne handlingen kan ikke angres."),
				"ar-SA": boldF("تحذير:", " ", "لا يمكن التراجع عن هذا الإجراء."),
			},
		},
		{
			source: boldF("Your order ", "#12345", " has been confirmed."),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldF("Ihre Bestellung ", "#12345", " wurde bestätigt."),
				"fr-FR": boldF("Votre commande ", "#12345", " a été confirmée."),
				"ja-JP": boldF("ご注文 ", "#12345", " が確認されました。"),
				"nb-NO": boldF("Bestillingen din ", "#12345", " er bekreftet."),
				"ar-SA": boldF("تم تأكيد طلبك ", "#12345", "."),
			},
		},
		// --- Structural entries (links) ---
		{
			source: linkF("Visit our ", "Help Center", " for more information."),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": linkF("Besuchen Sie unser ", "Hilfezentrum", " für weitere Informationen."),
				"fr-FR": linkF("Visitez notre ", "Centre d'aide", " pour plus d'informations."),
				"ja-JP": linkF("詳しくは", "ヘルプセンター", "をご覧ください。"),
				"nb-NO": linkF("Besøk ", "hjelpesenteret", " vårt for mer informasjon."),
				"ar-SA": linkF("قم بزيارة ", "مركز المساعدة", " لمزيد من المعلومات."),
			},
		},
		{
			source: linkF("Read our ", "Terms of Service", " before continuing."),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": linkF("Lesen Sie unsere ", "Nutzungsbedingungen", ", bevor Sie fortfahren."),
				"fr-FR": linkF("Lisez nos ", "Conditions d'utilisation", " avant de continuer."),
				"ja-JP": linkF("続行する前に", "利用規約", "をお読みください。"),
				"nb-NO": linkF("Les ", "vilkårene for bruk", " før du fortsetter."),
				"ar-SA": linkF("اقرأ ", "شروط الخدمة", " قبل المتابعة."),
			},
		},
		{
			source: linkF("Contact ", "Customer Support", " if you need assistance."),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": linkF("Kontaktieren Sie den ", "Kundendienst", ", wenn Sie Hilfe benötigen."),
				"fr-FR": linkF("Contactez le ", "Service client", " si vous avez besoin d'aide."),
				"ja-JP": linkF("サポートが必要な場合は", "カスタマーサポート", "にお問い合わせください。"),
				"nb-NO": linkF("Kontakt ", "kundestøtte", " hvis du trenger hjelp."),
				"ar-SA": linkF("تواصل مع ", "دعم العملاء", " إذا كنت بحاجة إلى مساعدة."),
			},
		},
		{
			source: linkF("Download the ", "SDK documentation", " to get started."),
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": linkF("Laden Sie die ", "SDK-Dokumentation", " herunter, um zu beginnen."),
				"fr-FR": linkF("Téléchargez la ", "documentation du SDK", " pour commencer."),
				"ja-JP": linkF("開始するには", "SDKドキュメント", "をダウンロードしてください。"),
				"nb-NO": linkF("Last ned ", "SDK-dokumentasjonen", " for å komme i gang."),
				"ar-SA": linkF("قم بتنزيل ", "وثائق SDK", " للبدء."),
			},
		},
		// --- Entity entries (person) ---
		{
			source:   entityF("Dear ", "person", "John", ", your order has shipped."),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityPerson, SourceValue: "John"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": entityF("Sehr geehrte/r ", "person", "John", ", Ihre Bestellung wurde versandt."),
				"fr-FR": entityF("Cher/Chère ", "person", "John", ", votre commande a été expédiée."),
				"ja-JP": entityF("", "person", "John", " 様、ご注文が発送されました。"),
				"nb-NO": entityF("Kjære ", "person", "John", ", bestillingen din er sendt."),
				"ar-SA": entityF("عزيزي ", "person", "John", "، تم شحن طلبك."),
			},
		},
		{
			source:   entityF("Hi ", "person", "Sarah", ", welcome to KapiMart!"),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityPerson, SourceValue: "Sarah"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": entityF("Hallo ", "person", "Sarah", ", willkommen bei KapiMart!"),
				"fr-FR": entityF("Bonjour ", "person", "Sarah", ", bienvenue sur KapiMart !"),
				"ja-JP": entityF("こんにちは ", "person", "Sarah", " さん、KapiMartへようこそ！"),
				"nb-NO": entityF("Hei ", "person", "Sarah", ", velkommen til KapiMart!"),
				"ar-SA": entityF("مرحبًا ", "person", "Sarah", "، مرحبًا بك في KapiMart!"),
			},
		},
		{
			source:   entityF("Thank you, ", "person", "Alex", ". Your review has been submitted."),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityPerson, SourceValue: "Alex"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": entityF("Vielen Dank, ", "person", "Alex", ". Ihre Bewertung wurde eingereicht."),
				"fr-FR": entityF("Merci, ", "person", "Alex", ". Votre avis a été soumis."),
				"ja-JP": entityF("ありがとうございます、", "person", "Alex", " さん。レビューが送信されました。"),
				"nb-NO": entityF("Takk, ", "person", "Alex", ". Anmeldelsen din er sendt inn."),
				"ar-SA": entityF("شكرًا لك، ", "person", "Alex", ". تم تقديم تقييمك."),
			},
		},
		// --- Entity entries (product) ---
		{
			source:   entityF("The ", "product", "Wireless Headphones", " are now back in stock."),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityProduct, SourceValue: "Wireless Headphones"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": entityF("Die ", "product", "Kabellose Kopfhörer", " sind wieder verfügbar."),
				"fr-FR": entityF("Les ", "product", "Écouteurs sans fil", " sont de nouveau en stock."),
				"ja-JP": entityF("", "product", "ワイヤレスヘッドフォン", " の在庫が補充されました。"),
				"nb-NO": entityF("", "product", "Trådløse hodetelefoner", " er igjen på lager."),
				"ar-SA": entityF("عاد ", "product", "سماعات لاسلكية", " إلى المخزون."),
			},
		},
		{
			source:   entityF("You saved $20 on ", "product", "Smart Home Hub", "!"),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityProduct, SourceValue: "Smart Home Hub"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": entityF("Sie haben 20 $ beim ", "product", "Smart Home Hub", " gespart!"),
				"fr-FR": entityF("Vous avez économisé 20 $ sur le ", "product", "Hub domotique", " !"),
				"ja-JP": entityF("", "product", "スマートホームハブ", " で20ドルお得です！"),
				"nb-NO": entityF("Du sparte 20 $ på ", "product", "Smart Home Hub", "!"),
				"ar-SA": entityF("وفرت 20 دولارًا على ", "product", "Smart Home Hub", "!"),
			},
		},
		// --- Entity entries (organization) ---
		{
			source:   entityF("Shipped by ", "organization", "FastPost", " via express delivery."),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityOrganization, SourceValue: "FastPost"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": entityF("Versandt durch ", "organization", "FastPost", " per Expresslieferung."),
				"fr-FR": entityF("Expédié par ", "organization", "FastPost", " en livraison express."),
				"ja-JP": entityF("", "organization", "FastPost", " による速達便で発送されました。"),
				"nb-NO": entityF("Sendt av ", "organization", "FastPost", " via ekspresslevering."),
				"ar-SA": entityF("تم الشحن بواسطة ", "organization", "FastPost", " عبر التوصيل السريع."),
			},
		},
		// --- Entity entries (currency) ---
		{
			source:   entityF("Your refund of ", "currency", "$49.99", " has been processed."),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityCurrency, SourceValue: "$49.99"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": entityF("Ihre Erstattung von ", "currency", "49,99 $", " wurde verarbeitet."),
				"fr-FR": entityF("Votre remboursement de ", "currency", "49,99 $", " a été traité."),
				"ja-JP": entityF("", "currency", "49.99ドル", " の返金が処理されました。"),
				"nb-NO": entityF("Refusjonen din på ", "currency", "49,99 $", " er behandlet."),
				"ar-SA": entityF("تمت معالجة استرداد ", "currency", "49.99 دولار", "."),
			},
		},
		// --- Combined: bold + entity ---
		{
			source:   boldEntityF("Hi ", "there", "! Your ", "product", "Travel Backpack", " is on its way."),
			entities: []enrichedEntity{{PlaceholderID: "2", Type: model.EntityProduct, SourceValue: "Travel Backpack"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldEntityF("Hallo", "", "! Ihr ", "product", "Reiserucksack", " ist unterwegs."),
				"fr-FR": boldEntityF("Bonjour", "", " ! Votre ", "product", "Sac à dos de voyage", " est en route."),
				"ja-JP": boldEntityF("こんにちは", "", "！ご注文の", "product", "トラベルバックパック", "は配送中です。"),
				"nb-NO": boldEntityF("Hei", "", "! Din ", "product", "Reiseryggsekk", " er på vei."),
				"ar-SA": boldEntityF("مرحبًا", "", "! منتج ", "product", "حقيبة سفر", " في الطريق إليك."),
			},
		},
		{
			source:   boldEntityF("Dear ", "Customer", ", ", "organization", "KapiMart", " values your feedback."),
			entities: []enrichedEntity{{PlaceholderID: "2", Type: model.EntityOrganization, SourceValue: "KapiMart"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldEntityF("Sehr geehrter ", "Kunde", ", ", "organization", "KapiMart", " schätzt Ihr Feedback."),
				"fr-FR": boldEntityF("Cher ", "Client", ", ", "organization", "KapiMart", " apprécie vos commentaires."),
				"ja-JP": boldEntityF("お客様", "各位", "、", "organization", "KapiMart", " はお客様のご意見を大切にしています。"),
				"nb-NO": boldEntityF("Kjære ", "kunde", ", ", "organization", "KapiMart", " setter pris på tilbakemeldingene dine."),
				"ar-SA": boldEntityF("عزيزي ", "العميل", "، ", "organization", "KapiMart", " تقدر ملاحظاتك."),
			},
		},
		{
			source:   boldEntityF("Order ", "confirmed", " for ", "person", "Emily", ". Check your email for details."),
			entities: []enrichedEntity{{PlaceholderID: "2", Type: model.EntityPerson, SourceValue: "Emily"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldEntityF("Bestellung ", "bestätigt", " für ", "person", "Emily", ". Details finden Sie in Ihrer E-Mail."),
				"fr-FR": boldEntityF("Commande ", "confirmée", " pour ", "person", "Emily", ". Consultez votre e-mail pour les détails."),
				"ja-JP": boldEntityF("注文", "確認済み", " — ", "person", "Emily", " さん、詳細はメールをご確認ください。"),
				"nb-NO": boldEntityF("Bestilling ", "bekreftet", " for ", "person", "Emily", ". Sjekk e-posten din for detaljer."),
				"ar-SA": boldEntityF("تم ", "تأكيد الطلب", " لـ ", "person", "Emily", ". تحقق من بريدك الإلكتروني للتفاصيل."),
			},
		},
		{
			source:   boldEntityF("", "Flash Sale", ": Save big on ", "product", "Fitness Tracker Watch", " today!"),
			entities: []enrichedEntity{{PlaceholderID: "2", Type: model.EntityProduct, SourceValue: "Fitness Tracker Watch"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de-DE": boldEntityF("", "Blitzangebot", ": Sparen Sie heute beim ", "product", "Fitness-Tracker", "!"),
				"fr-FR": boldEntityF("", "Vente flash", " : Profitez de la ", "product", "Montre connectée", " aujourd'hui !"),
				"ja-JP": boldEntityF("", "タイムセール", "：本日の", "product", "フィットネストラッカー", "がお買い得！"),
				"nb-NO": boldEntityF("", "Lynkupp", ": Spar stort på ", "product", "Aktivitetsmåler", " i dag!"),
				"ar-SA": boldEntityF("", "تخفيضات خاطفة", ": وفر على ", "product", "ساعة تتبع اللياقة", " اليوم!"),
			},
		},
	}
}

// --- Utility ---

// spreadTimestamps distributes created_at timestamps across the past `days`
// days so sample data produces a realistic activity chart. Each row gets a
// random date within the window, with a bias toward more recent dates.
func spreadTimestamps(db *storage.DB, table string, days int) {
	rows, err := db.Query(fmt.Sprintf("SELECT id FROM %s ORDER BY RANDOM()", table))
	if err != nil {
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return
	}

	now := time.Now()
	rng := rand.New(rand.NewSource(42)) // deterministic for reproducibility
	for _, id := range ids {
		// Bias toward recent: square the random value so more entries cluster near today.
		daysAgo := int(float64(days) * rng.Float64() * rng.Float64())
		ts := now.AddDate(0, 0, -daysAgo).Format(time.RFC3339)
		_, _ = db.Exec(
			fmt.Sprintf("UPDATE %s SET created_at = ?, updated_at = ? WHERE id = ?", table),
			ts, ts, id,
		)
	}
}

func copyEmbeddedDir(srcDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(assetsFS, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, err := assetsFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
}
