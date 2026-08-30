// Package sample provides the embedded sample project for the kapi-desktop
// app: KapiMart, a governed multi-collection project in the natural per-area
// layout (web/src/legal/marketing with locale dirs beside source), shipping the
// committed context its recipe binds.
package sample

import (
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
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// `all:` is required: the sample commits its `.kapi/` context — the voice
// profile, the terms record, the content memory and the unit-state ledger — and
// a plain pattern excludes every name beginning with a dot.
//
//go:embed all:kapimart
var assetsFS embed.FS

// DisplayName maps an internal sample name to its user-facing name.
var DisplayName = map[string]string{
	"kapimart": "KapiMart",
}

// List returns the available sample project names.
func List() []string {
	return []string{"kapimart"}
}

// Scaffold creates a sample project on disk at targetDir.
// name must be "kapimart".
func Scaffold(name, targetDir string) error {
	if _, ok := DisplayName[name]; !ok {
		return fmt.Errorf("unknown sample project %q", name)
	}

	// Copy source files: natural per-area layout (source under <area>/en/,
	// localized files beside it under sibling locale dirs — no separate
	// output/ tree).
	for _, area := range []string{"web", "src", "legal", "marketing"} {
		if err := copyEmbeddedDir("kapimart/"+area, filepath.Join(targetDir, area)); err != nil {
			return fmt.Errorf("copy %s files: %w", area, err)
		}
	}

	// Copy the committed context: the voice profile, the terms record and the
	// content memory the recipe binds. They land on disk as authored files, the
	// same ones `git diff` would review in a real project, and the store is
	// compiled from them below rather than from a second copy.
	if err := copyEmbeddedDir("kapimart/"+project.StateDirName, filepath.Join(targetDir, project.StateDirName)); err != nil {
		return fmt.Errorf("copy context files: %w", err)
	}

	// Copy the project recipe (kapi.yaml).
	kapiData, err := assetsFS.ReadFile(name + "/kapi.yaml")
	if err != nil {
		return fmt.Errorf("read kapi.yaml: %w", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create project dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "kapi.yaml"), kapiData, 0o644); err != nil {
		return fmt.Errorf("write kapi.yaml: %w", err)
	}

	// Seed the content memory and terms into the project's own store. Both are
	// schemas of `.kapi/work/store.db`, so this is one handle rather than two files —
	// and the handle must be closed before the caller opens the project, or the
	// process would hold two connection pools on it.
	if err := seedStore(targetDir); err != nil {
		return err
	}

	// Stamp the sample manifest (.kapi/sample.json) so the desktop can detect an
	// out-of-date scaffolded copy on disk and offer to refresh it.
	if err := writeManifest(name, targetDir); err != nil {
		return fmt.Errorf("write sample manifest: %w", err)
	}

	return nil
}

// --- KapiMart seed functions ---

var v2Targets = []model.LocaleID{"de", "fr", "ja", "nb", "ar"}

// seedStore opens the sample project's store, seeds the content memory and the
// terms, and closes it. Opening creates `.kapi/` and the store file, so nothing
// needs to make the state directory first.
func seedStore(targetDir string) error {
	layout := project.Layout{
		Root:     targetDir,
		StateDir: filepath.Join(targetDir, project.StateDirName),
	}
	db, err := projectdb.Open(context.Background(), layout)
	if err != nil {
		return fmt.Errorf("open sample project store: %w", err)
	}
	defer db.Close()

	if err := seedTMv2(db.Memory(), targetDir); err != nil {
		return fmt.Errorf("seed content memory: %w", err)
	}
	if err := seedTermsv2(db.Terms(), targetDir); err != nil {
		return fmt.Errorf("seed terms: %w", err)
	}
	return nil
}

// MemorySourceRel and TermsSourceRel are where the committed context sources
// sit inside a scaffolded project. They are the paths `kapi.yaml` binds under
// defaults.memory_source and defaults.terms_source, so the store is compiled
// from the same files the recipe names.
var (
	MemorySourceRel = filepath.Join(project.StateDirName, project.MemoryDirName, kmb.ConventionalName)
	TermsSourceRel  = filepath.Join(project.StateDirName, ktb.ConventionalName)
)

func seedTMv2(tm *memory.SQLiteStore, targetDir string) error {
	ctx := context.Background()

	// Compiled through the same importer `kapi apply` uses, from the file the
	// recipe binds, so the store has one writer and the committed bundle is the
	// only description of what is in it.
	if _, err := host.ImportKMBFile(ctx, tm, filepath.Join(targetDir, MemorySourceRel)); err != nil {
		return fmt.Errorf("compile content memory: %w", err)
	}

	// Add enriched entries with structural inline codes and entity annotations.
	if err := seedEnrichedEntries(tm); err != nil {
		return fmt.Errorf("seed enriched entries: %w", err)
	}

	// The bulk path skips the per-row FTS5 inserts, leaving the search and fuzzy
	// side-tables empty until they are rebuilt set-wise. Exact lookup works
	// without this; search and fuzzy lookup return nothing and report no error.
	if err := tm.RebuildSearchIndex(ctx); err != nil {
		return fmt.Errorf("rebuild content-memory search index: %w", err)
	}
	if err := tm.RebuildFuzzyIndex(ctx); err != nil {
		return fmt.Errorf("rebuild content-memory fuzzy index: %w", err)
	}

	// Spread timestamps over 90 days for a realistic activity chart.
	spreadTimestamps(tm.DB(), "tm_entries", 90)
	return nil
}

func seedTermsv2(tb *terms.SQLiteStore, targetDir string) error {
	f, err := os.Open(filepath.Join(targetDir, TermsSourceRel))
	if err != nil {
		return fmt.Errorf("open terms source: %w", err)
	}
	defer f.Close()

	if _, err := host.ImportKTBFile(context.Background(), tb, f); err != nil {
		return fmt.Errorf("compile terms: %w", err)
	}
	spreadTimestamps(tb.DB(), "tb_concepts", 90)
	return nil
}

// --- Enriched content-memory entries (structural + entity) ---

// enrichedEntry defines a content-memory entry with inline codes and/or entity placeholders.
// The source is always in en; targets maps each supported locale to a
// Run-sequence factory. Entities, when set, carry the placeholder ID, type,
// and the en value; per-locale entity values are not defined separately
// for sample data.
type enrichedEntry struct {
	source   func() []model.Run
	targets  map[model.LocaleID]func() []model.Run
	entities []enrichedEntity
}

// enrichedEntity is the sample-file shape for an entity mapping — just
// the placeholder ID, type, and the en value. At seed time we expand
// this into a memory.EntityMapping with a Values map keyed by en.
type enrichedEntity struct {
	PlaceholderID string
	Type          model.EntityType
	SourceValue   string
}

// seedEnrichedEntries adds multilingual content-memory entries with structural markup
// and entity annotations that exercise all 6 match tiers. Each definition
// produces exactly one entry with en as the canonical source and all
// v2Targets as peer variants.
func seedEnrichedEntries(tm *memory.SQLiteStore) error {
	entries := enrichedEntryDefs()
	now := time.Now()
	for i, def := range entries {
		variants := map[model.LocaleID][]model.Run{
			"en": def.source(),
		}
		for _, tgt := range v2Targets {
			if fn, ok := def.targets[tgt]; ok {
				variants[tgt] = fn()
			}
		}
		entity := make([]memory.EntityMapping, 0, len(def.entities))
		for _, e := range def.entities {
			entity = append(entity, memory.EntityMapping{
				PlaceholderID: e.PlaceholderID,
				Type:          e.Type,
				Values: map[model.LocaleID]memory.EntityValue{
					"en": {Text: e.SourceValue},
				},
			})
		}
		entry := memory.Entry{
			ID:          id.New(),
			Variants:    variants,
			HintSrcLang: "en",
			Entities:    entity,
			Origins: []memory.Origin{
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
				"de": boldF("Klicken Sie ", "hier", ", um Ihre Bestellung anzuzeigen."),
				"fr": boldF("Cliquez ", "ici", " pour voir votre commande."),
				"ja": boldF("注文を表示するには", "こちら", "をクリックしてください。"),
				"nb": boldF("Klikk ", "her", " for å se bestillingen din."),
				"ar": boldF("انقر ", "هنا", " لعرض طلبك."),
			},
		},
		{
			source: boldF("Your ", "payment", " has been processed successfully."),
			targets: map[model.LocaleID]func() []model.Run{
				"de": boldF("Ihre ", "Zahlung", " wurde erfolgreich verarbeitet."),
				"fr": boldF("Votre ", "paiement", " a été traité avec succès."),
				"ja": boldF("お", "支払い", "は正常に処理されました。"),
				"nb": boldF("Din ", "betaling", " er behandlet."),
				"ar": boldF("تمت معالجة ", "الدفع", " بنجاح."),
			},
		},
		{
			source: boldF("Free shipping", "", " on all orders over $50!"),
			targets: map[model.LocaleID]func() []model.Run{
				"de": boldF("Kostenloser Versand", "", " für alle Bestellungen über 50 $!"),
				"fr": boldF("Livraison gratuite", "", " pour toutes les commandes de plus de 50 $ !"),
				"ja": boldF("送料無料", "", " — 50ドル以上のご注文が対象です！"),
				"nb": boldF("Gratis frakt", "", " på alle bestillinger over 50 $!"),
				"ar": boldF("شحن مجاني", "", " على جميع الطلبات التي تزيد عن 50 دولار!"),
			},
		},
		{
			source: boldF("Important:", " ", "Your account will be deactivated in 30 days."),
			targets: map[model.LocaleID]func() []model.Run{
				"de": boldF("Wichtig:", " ", "Ihr Konto wird in 30 Tagen deaktiviert."),
				"fr": boldF("Important :", " ", "Votre compte sera désactivé dans 30 jours."),
				"ja": boldF("重要：", "", "アカウントは30日後に無効になります。"),
				"nb": boldF("Viktig:", " ", "Kontoen din deaktiveres om 30 dager."),
				"ar": boldF("مهم:", " ", "سيتم إلغاء تفعيل حسابك خلال 30 يومًا."),
			},
		},
		{
			source: boldF("New!", " ", "Check out our summer collection."),
			targets: map[model.LocaleID]func() []model.Run{
				"de": boldF("Neu!", " ", "Entdecken Sie unsere Sommerkollektion."),
				"fr": boldF("Nouveau !", " ", "Découvrez notre collection d'été."),
				"ja": boldF("新着！", "", "サマーコレクションをご覧ください。"),
				"nb": boldF("Nytt!", " ", "Sjekk ut sommersamlingen vår."),
				"ar": boldF("جديد!", " ", "اطلع على مجموعة الصيف."),
			},
		},
		{
			source: boldF("Save 20%", "", " when you subscribe to our newsletter."),
			targets: map[model.LocaleID]func() []model.Run{
				"de": boldF("Sparen Sie 20 %", "", ", wenn Sie unseren Newsletter abonnieren."),
				"fr": boldF("Économisez 20 %", "", " en vous abonnant à notre newsletter."),
				"ja": boldF("20% 割引", "", " — ニュースレターに登録するとお得です。"),
				"nb": boldF("Spar 20 %", "", " når du abonnerer på nyhetsbrevet vårt."),
				"ar": boldF("وفر 20%", "", " عند الاشتراك في النشرة الإخبارية."),
			},
		},
		{
			source: boldF("Warning:", " ", "This action cannot be undone."),
			targets: map[model.LocaleID]func() []model.Run{
				"de": boldF("Warnung:", " ", "Diese Aktion kann nicht rückgängig gemacht werden."),
				"fr": boldF("Attention :", " ", "Cette action est irréversible."),
				"ja": boldF("警告：", "", "この操作は元に戻せません。"),
				"nb": boldF("Advarsel:", " ", "Denne handlingen kan ikke angres."),
				"ar": boldF("تحذير:", " ", "لا يمكن التراجع عن هذا الإجراء."),
			},
		},
		{
			source: boldF("Your order ", "#12345", " has been confirmed."),
			targets: map[model.LocaleID]func() []model.Run{
				"de": boldF("Ihre Bestellung ", "#12345", " wurde bestätigt."),
				"fr": boldF("Votre commande ", "#12345", " a été confirmée."),
				"ja": boldF("ご注文 ", "#12345", " が確認されました。"),
				"nb": boldF("Bestillingen din ", "#12345", " er bekreftet."),
				"ar": boldF("تم تأكيد طلبك ", "#12345", "."),
			},
		},
		// --- Structural entries (links) ---
		{
			source: linkF("Visit our ", "Help Center", " for more information."),
			targets: map[model.LocaleID]func() []model.Run{
				"de": linkF("Besuchen Sie unser ", "Hilfezentrum", " für weitere Informationen."),
				"fr": linkF("Visitez notre ", "Centre d'aide", " pour plus d'informations."),
				"ja": linkF("詳しくは", "ヘルプセンター", "をご覧ください。"),
				"nb": linkF("Besøk ", "hjelpesenteret", " vårt for mer informasjon."),
				"ar": linkF("قم بزيارة ", "مركز المساعدة", " لمزيد من المعلومات."),
			},
		},
		{
			source: linkF("Read our ", "Terms of Service", " before continuing."),
			targets: map[model.LocaleID]func() []model.Run{
				"de": linkF("Lesen Sie unsere ", "Nutzungsbedingungen", ", bevor Sie fortfahren."),
				"fr": linkF("Lisez nos ", "Conditions d'utilisation", " avant de continuer."),
				"ja": linkF("続行する前に", "利用規約", "をお読みください。"),
				"nb": linkF("Les ", "vilkårene for bruk", " før du fortsetter."),
				"ar": linkF("اقرأ ", "شروط الخدمة", " قبل المتابعة."),
			},
		},
		{
			source: linkF("Contact ", "Customer Support", " if you need assistance."),
			targets: map[model.LocaleID]func() []model.Run{
				"de": linkF("Kontaktieren Sie den ", "Kundendienst", ", wenn Sie Hilfe benötigen."),
				"fr": linkF("Contactez le ", "Service client", " si vous avez besoin d'aide."),
				"ja": linkF("サポートが必要な場合は", "カスタマーサポート", "にお問い合わせください。"),
				"nb": linkF("Kontakt ", "kundestøtte", " hvis du trenger hjelp."),
				"ar": linkF("تواصل مع ", "دعم العملاء", " إذا كنت بحاجة إلى مساعدة."),
			},
		},
		{
			source: linkF("Download the ", "SDK documentation", " to get started."),
			targets: map[model.LocaleID]func() []model.Run{
				"de": linkF("Laden Sie die ", "SDK-Dokumentation", " herunter, um zu beginnen."),
				"fr": linkF("Téléchargez la ", "documentation du SDK", " pour commencer."),
				"ja": linkF("開始するには", "SDKドキュメント", "をダウンロードしてください。"),
				"nb": linkF("Last ned ", "SDK-dokumentasjonen", " for å komme i gang."),
				"ar": linkF("قم بتنزيل ", "وثائق SDK", " للبدء."),
			},
		},
		// --- Entity entries (person) ---
		{
			source:   entityF("Dear ", "person", "John", ", your order has shipped."),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityPerson, SourceValue: "John"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de": entityF("Sehr geehrte/r ", "person", "John", ", Ihre Bestellung wurde versandt."),
				"fr": entityF("Cher/Chère ", "person", "John", ", votre commande a été expédiée."),
				"ja": entityF("", "person", "John", " 様、ご注文が発送されました。"),
				"nb": entityF("Kjære ", "person", "John", ", bestillingen din er sendt."),
				"ar": entityF("عزيزي ", "person", "John", "، تم شحن طلبك."),
			},
		},
		{
			source:   entityF("Hi ", "person", "Sarah", ", welcome to KapiMart!"),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityPerson, SourceValue: "Sarah"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de": entityF("Hallo ", "person", "Sarah", ", willkommen bei KapiMart!"),
				"fr": entityF("Bonjour ", "person", "Sarah", ", bienvenue sur KapiMart !"),
				"ja": entityF("こんにちは ", "person", "Sarah", " さん、KapiMartへようこそ！"),
				"nb": entityF("Hei ", "person", "Sarah", ", velkommen til KapiMart!"),
				"ar": entityF("مرحبًا ", "person", "Sarah", "، مرحبًا بك في KapiMart!"),
			},
		},
		{
			source:   entityF("Thank you, ", "person", "Alex", ". Your review has been submitted."),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityPerson, SourceValue: "Alex"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de": entityF("Vielen Dank, ", "person", "Alex", ". Ihre Bewertung wurde eingereicht."),
				"fr": entityF("Merci, ", "person", "Alex", ". Votre avis a été soumis."),
				"ja": entityF("ありがとうございます、", "person", "Alex", " さん。レビューが送信されました。"),
				"nb": entityF("Takk, ", "person", "Alex", ". Anmeldelsen din er sendt inn."),
				"ar": entityF("شكرًا لك، ", "person", "Alex", ". تم تقديم تقييمك."),
			},
		},
		// --- Entity entries (product) ---
		{
			source:   entityF("The ", "product", "Wireless Headphones", " are now back in stock."),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityProduct, SourceValue: "Wireless Headphones"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de": entityF("Die ", "product", "Kabellose Kopfhörer", " sind wieder verfügbar."),
				"fr": entityF("Les ", "product", "Écouteurs sans fil", " sont de nouveau en stock."),
				"ja": entityF("", "product", "ワイヤレスヘッドフォン", " の在庫が補充されました。"),
				"nb": entityF("", "product", "Trådløse hodetelefoner", " er igjen på lager."),
				"ar": entityF("عاد ", "product", "سماعات لاسلكية", " إلى المخزون."),
			},
		},
		{
			source:   entityF("You saved $20 on ", "product", "Smart Home Hub", "!"),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityProduct, SourceValue: "Smart Home Hub"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de": entityF("Sie haben 20 $ beim ", "product", "Smart Home Hub", " gespart!"),
				"fr": entityF("Vous avez économisé 20 $ sur le ", "product", "Hub domotique", " !"),
				"ja": entityF("", "product", "スマートホームハブ", " で20ドルお得です！"),
				"nb": entityF("Du sparte 20 $ på ", "product", "Smart Home Hub", "!"),
				"ar": entityF("وفرت 20 دولارًا على ", "product", "Smart Home Hub", "!"),
			},
		},
		// --- Entity entries (organization) ---
		{
			source:   entityF("Shipped by ", "organization", "FastPost", " via express delivery."),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityOrganization, SourceValue: "FastPost"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de": entityF("Versandt durch ", "organization", "FastPost", " per Expresslieferung."),
				"fr": entityF("Expédié par ", "organization", "FastPost", " en livraison express."),
				"ja": entityF("", "organization", "FastPost", " による速達便で発送されました。"),
				"nb": entityF("Sendt av ", "organization", "FastPost", " via ekspresslevering."),
				"ar": entityF("تم الشحن بواسطة ", "organization", "FastPost", " عبر التوصيل السريع."),
			},
		},
		// --- Entity entries (currency) ---
		{
			source:   entityF("Your refund of ", "currency", "$49.99", " has been processed."),
			entities: []enrichedEntity{{PlaceholderID: "1", Type: model.EntityCurrency, SourceValue: "$49.99"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de": entityF("Ihre Erstattung von ", "currency", "49,99 $", " wurde verarbeitet."),
				"fr": entityF("Votre remboursement de ", "currency", "49,99 $", " a été traité."),
				"ja": entityF("", "currency", "49.99ドル", " の返金が処理されました。"),
				"nb": entityF("Refusjonen din på ", "currency", "49,99 $", " er behandlet."),
				"ar": entityF("تمت معالجة استرداد ", "currency", "49.99 دولار", "."),
			},
		},
		// --- Combined: bold + entity ---
		{
			source:   boldEntityF("Hi ", "there", "! Your ", "product", "Travel Backpack", " is on its way."),
			entities: []enrichedEntity{{PlaceholderID: "2", Type: model.EntityProduct, SourceValue: "Travel Backpack"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de": boldEntityF("Hallo", "", "! Ihr ", "product", "Reiserucksack", " ist unterwegs."),
				"fr": boldEntityF("Bonjour", "", " ! Votre ", "product", "Sac à dos de voyage", " est en route."),
				"ja": boldEntityF("こんにちは", "", "！ご注文の", "product", "トラベルバックパック", "は配送中です。"),
				"nb": boldEntityF("Hei", "", "! Din ", "product", "Reiseryggsekk", " er på vei."),
				"ar": boldEntityF("مرحبًا", "", "! منتج ", "product", "حقيبة سفر", " في الطريق إليك."),
			},
		},
		{
			source:   boldEntityF("Dear ", "Customer", ", ", "organization", "KapiMart", " values your feedback."),
			entities: []enrichedEntity{{PlaceholderID: "2", Type: model.EntityOrganization, SourceValue: "KapiMart"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de": boldEntityF("Sehr geehrter ", "Kunde", ", ", "organization", "KapiMart", " schätzt Ihr Feedback."),
				"fr": boldEntityF("Cher ", "Client", ", ", "organization", "KapiMart", " apprécie vos commentaires."),
				"ja": boldEntityF("お客様", "各位", "、", "organization", "KapiMart", " はお客様のご意見を大切にしています。"),
				"nb": boldEntityF("Kjære ", "kunde", ", ", "organization", "KapiMart", " setter pris på tilbakemeldingene dine."),
				"ar": boldEntityF("عزيزي ", "العميل", "، ", "organization", "KapiMart", " تقدر ملاحظاتك."),
			},
		},
		{
			source:   boldEntityF("Order ", "confirmed", " for ", "person", "Emily", ". Check your email for details."),
			entities: []enrichedEntity{{PlaceholderID: "2", Type: model.EntityPerson, SourceValue: "Emily"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de": boldEntityF("Bestellung ", "bestätigt", " für ", "person", "Emily", ". Details finden Sie in Ihrer E-Mail."),
				"fr": boldEntityF("Commande ", "confirmée", " pour ", "person", "Emily", ". Consultez votre e-mail pour les détails."),
				"ja": boldEntityF("注文", "確認済み", " — ", "person", "Emily", " さん、詳細はメールをご確認ください。"),
				"nb": boldEntityF("Bestilling ", "bekreftet", " for ", "person", "Emily", ". Sjekk e-posten din for detaljer."),
				"ar": boldEntityF("تم ", "تأكيد الطلب", " لـ ", "person", "Emily", ". تحقق من بريدك الإلكتروني للتفاصيل."),
			},
		},
		{
			source:   boldEntityF("", "Flash Sale", ": Save big on ", "product", "Fitness Tracker Watch", " today!"),
			entities: []enrichedEntity{{PlaceholderID: "2", Type: model.EntityProduct, SourceValue: "Fitness Tracker Watch"}},
			targets: map[model.LocaleID]func() []model.Run{
				"de": boldEntityF("", "Blitzangebot", ": Sparen Sie heute beim ", "product", "Fitness-Tracker", "!"),
				"fr": boldEntityF("", "Vente flash", " : Profitez de la ", "product", "Montre connectée", " aujourd'hui !"),
				"ja": boldEntityF("", "タイムセール", "：本日の", "product", "フィットネストラッカー", "がお買い得！"),
				"nb": boldEntityF("", "Lynkupp", ": Spar stort på ", "product", "Aktivitetsmåler", " i dag!"),
				"ar": boldEntityF("", "تخفيضات خاطفة", ": وفر على ", "product", "ساعة تتبع اللياقة", " اليوم!"),
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
