//go:build ignore
// +build ignore

// generate_samples creates sample .kaz project files for Bowrain testing.
// Run with: go run generate_samples.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gokapi/gokapi/core/kaz"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/lib/termbase"
)

func main() {
	fmt.Println("Generating Bowrain sample projects...")

	if err := generateWebsiteProject(); err != nil {
		fmt.Fprintf(os.Stderr, "website project: %v\n", err)
		os.Exit(1)
	}

	if err := generateSoftwareProject(); err != nil {
		fmt.Fprintf(os.Stderr, "software project: %v\n", err)
		os.Exit(1)
	}

	if err := generateMarketingProject(); err != nil {
		fmt.Fprintf(os.Stderr, "marketing project: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Done! Sample projects generated in apps/bowrain/samples/")
}

// generateWebsiteProject creates a half-translated website project with TM and terms.
func generateWebsiteProject() error {
	fmt.Println("  Creating: website-translation.kaz (half-translated, with TM & terms)")

	htmlSource := []byte(`<!DOCTYPE html>
<html lang="en">
<head><title>Acme Corp - Welcome</title></head>
<body>
<h1>Welcome to Acme Corporation</h1>
<p>We provide innovative solutions for modern businesses. Our platform helps teams collaborate effectively and deliver results faster.</p>
<p>Founded in 2020, Acme Corporation has grown to serve over 10,000 customers worldwide.</p>
<h2>Our Products</h2>
<p>Acme Cloud Platform enables seamless deployment of applications across multiple regions.</p>
<p>Acme Analytics provides real-time insights into your business performance.</p>
<p>Acme Security Suite protects your infrastructure with enterprise-grade security.</p>
<h2>Contact Us</h2>
<p>Ready to get started? Contact our sales team today.</p>
<p>Email: sales@acmecorp.com | Phone: +1 (555) 123-4567</p>
</body>
</html>`)

	parts := buildHTMLParts(htmlSource, []blockDef{
		{id: "h1", source: "Welcome to Acme Corporation", targetFR: "Bienvenue chez Acme Corporation", targetDE: ""},
		{id: "p1", source: "We provide innovative solutions for modern businesses. Our platform helps teams collaborate effectively and deliver results faster.", targetFR: "Nous fournissons des solutions innovantes pour les entreprises modernes. Notre plateforme aide les équipes à collaborer efficacement et à obtenir des résultats plus rapidement.", targetDE: ""},
		{id: "p2", source: "Founded in 2020, Acme Corporation has grown to serve over 10,000 customers worldwide.", targetFR: "Fondée en 2020, Acme Corporation a grandi pour servir plus de 10 000 clients dans le monde.", targetDE: "Acme Corporation wurde 2020 gegründet und betreut mittlerweile über 10.000 Kunden weltweit."},
		{id: "h2a", source: "Our Products", targetFR: "Nos Produits", targetDE: "Unsere Produkte"},
		{id: "p3", source: "Acme Cloud Platform enables seamless deployment of applications across multiple regions.", targetFR: "", targetDE: ""},
		{id: "p4", source: "Acme Analytics provides real-time insights into your business performance.", targetFR: "", targetDE: ""},
		{id: "p5", source: "Acme Security Suite protects your infrastructure with enterprise-grade security.", targetFR: "", targetDE: ""},
		{id: "h2b", source: "Contact Us", targetFR: "Contactez-nous", targetDE: "Kontaktieren Sie uns"},
		{id: "p6", source: "Ready to get started? Contact our sales team today.", targetFR: "", targetDE: ""},
		{id: "p7", source: "Email: sales@acmecorp.com | Phone: +1 (555) 123-4567", nonTranslatable: true},
	})

	// Build TM entries
	tmEntries := []tmEntry{
		{source: "Welcome to Acme Corporation", targetFR: "Bienvenue chez Acme Corporation", targetDE: "Willkommen bei Acme Corporation"},
		{source: "Our Products", targetFR: "Nos Produits", targetDE: "Unsere Produkte"},
		{source: "Contact Us", targetFR: "Contactez-nous", targetDE: "Kontaktieren Sie uns"},
		{source: "Ready to get started?", targetFR: "Prêt à commencer ?", targetDE: "Bereit loszulegen?"},
		{source: "Contact our sales team today.", targetFR: "Contactez notre équipe commerciale dès aujourd'hui.", targetDE: "Kontaktieren Sie noch heute unser Vertriebsteam."},
		{source: "innovative solutions", targetFR: "solutions innovantes", targetDE: "innovative Lösungen"},
		{source: "enterprise-grade security", targetFR: "sécurité de niveau entreprise", targetDE: "Sicherheit auf Unternehmensniveau"},
		{source: "real-time insights", targetFR: "informations en temps réel", targetDE: "Echtzeit-Einblicke"},
	}
	tmData := buildTMJSON(tmEntries)

	// Build termbase
	termsData := buildTermsJSON("Acme Corp", []conceptDef{
		{domain: "product", definition: "Cloud computing platform by Acme", terms: []termDef{
			{text: "Acme Cloud Platform", locale: "en", status: "preferred"},
			{text: "Plateforme Cloud Acme", locale: "fr", status: "preferred"},
			{text: "Acme Cloud-Plattform", locale: "de", status: "preferred"},
		}},
		{domain: "product", definition: "Analytics product by Acme", terms: []termDef{
			{text: "Acme Analytics", locale: "en", status: "preferred"},
			{text: "Acme Analytics", locale: "fr", status: "preferred"},
			{text: "Acme Analytics", locale: "de", status: "preferred"},
		}},
		{domain: "product", definition: "Security product by Acme", terms: []termDef{
			{text: "Acme Security Suite", locale: "en", status: "preferred"},
			{text: "Suite de Sécurité Acme", locale: "fr", status: "preferred"},
			{text: "Acme Sicherheitspaket", locale: "de", status: "preferred"},
		}},
		{domain: "general", definition: "Enterprise-level security protection", terms: []termDef{
			{text: "enterprise-grade security", locale: "en", status: "preferred"},
			{text: "sécurité de niveau entreprise", locale: "fr", status: "preferred"},
			{text: "Sicherheit auf Unternehmensniveau", locale: "de", status: "preferred"},
		}},
		{domain: "general", definition: "Instant data analysis capabilities", terms: []termDef{
			{text: "real-time insights", locale: "en", status: "preferred"},
			{text: "informations en temps réel", locale: "fr", status: "preferred"},
			{text: "Echtzeit-Einblicke", locale: "de", status: "preferred"},
		}},
		{domain: "brand", definition: "Company name - do not translate", terms: []termDef{
			{text: "Acme Corporation", locale: "en", status: "preferred"},
			{text: "Acme Corporation", locale: "fr", status: "preferred", note: "Do not translate company name"},
			{text: "Acme Corporation", locale: "de", status: "preferred", note: "Firmenname nicht übersetzen"},
		}},
	})

	return writeKAZ("apps/bowrain/samples/website-translation.kaz", kaz.PackOptions{
		Name:          "Acme Website",
		SourceLocale:  "en",
		TargetLocales: []string{"fr", "de"},
		Items: []kaz.PackItem{
			{Name: "index.html", Format: "html", Type: "file", SourceBytes: htmlSource, Parts: parts},
		},
		TMData:    tmData,
		TermsData: termsData,
	})
}

// generateSoftwareProject creates a software UI strings project with JSON.
func generateSoftwareProject() error {
	fmt.Println("  Creating: software-ui.kaz (new project, with large TM & terms)")

	jsonSource := []byte(`{
  "app": {
    "title": "Task Manager Pro",
    "menu": {
      "file": "File",
      "edit": "Edit",
      "view": "View",
      "help": "Help"
    },
    "actions": {
      "save": "Save",
      "save_as": "Save As...",
      "open": "Open",
      "close": "Close",
      "undo": "Undo",
      "redo": "Redo",
      "cut": "Cut",
      "copy": "Copy",
      "paste": "Paste",
      "delete": "Delete",
      "select_all": "Select All",
      "find": "Find...",
      "replace": "Find and Replace..."
    },
    "dialogs": {
      "confirm_delete": "Are you sure you want to delete this item?",
      "unsaved_changes": "You have unsaved changes. Do you want to save before closing?",
      "save_success": "Your changes have been saved successfully.",
      "error_generic": "An unexpected error occurred. Please try again.",
      "no_internet": "No internet connection. Please check your network settings.",
      "session_expired": "Your session has expired. Please log in again."
    },
    "tasks": {
      "new_task": "New Task",
      "due_date": "Due Date",
      "priority": "Priority",
      "status": "Status",
      "assignee": "Assignee",
      "description": "Description",
      "comments": "Comments",
      "attachments": "Attachments",
      "tags": "Tags"
    }
  }
}`)

	parts := buildJSONParts([]blockDef{
		{id: "title", source: "Task Manager Pro", nonTranslatable: true},
		{id: "menu.file", source: "File"},
		{id: "menu.edit", source: "Edit"},
		{id: "menu.view", source: "View"},
		{id: "menu.help", source: "Help"},
		{id: "actions.save", source: "Save"},
		{id: "actions.save_as", source: "Save As..."},
		{id: "actions.open", source: "Open"},
		{id: "actions.close", source: "Close"},
		{id: "actions.undo", source: "Undo"},
		{id: "actions.redo", source: "Redo"},
		{id: "actions.cut", source: "Cut"},
		{id: "actions.copy", source: "Copy"},
		{id: "actions.paste", source: "Paste"},
		{id: "actions.delete", source: "Delete"},
		{id: "actions.select_all", source: "Select All"},
		{id: "actions.find", source: "Find..."},
		{id: "actions.replace", source: "Find and Replace..."},
		{id: "dialogs.confirm_delete", source: "Are you sure you want to delete this item?"},
		{id: "dialogs.unsaved_changes", source: "You have unsaved changes. Do you want to save before closing?"},
		{id: "dialogs.save_success", source: "Your changes have been saved successfully."},
		{id: "dialogs.error_generic", source: "An unexpected error occurred. Please try again."},
		{id: "dialogs.no_internet", source: "No internet connection. Please check your network settings."},
		{id: "dialogs.session_expired", source: "Your session has expired. Please log in again."},
		{id: "tasks.new_task", source: "New Task"},
		{id: "tasks.due_date", source: "Due Date"},
		{id: "tasks.priority", source: "Priority"},
		{id: "tasks.status", source: "Status"},
		{id: "tasks.assignee", source: "Assignee"},
		{id: "tasks.description", source: "Description"},
		{id: "tasks.comments", source: "Comments"},
		{id: "tasks.attachments", source: "Attachments"},
		{id: "tasks.tags", source: "Tags"},
	})

	// Large TM with common software strings (from "previous projects")
	tmEntries := []tmEntry{
		{source: "File", targetFR: "Fichier", targetDE: "Datei", targetJA: "ファイル"},
		{source: "Edit", targetFR: "Modifier", targetDE: "Bearbeiten", targetJA: "編集"},
		{source: "View", targetFR: "Affichage", targetDE: "Ansicht", targetJA: "表示"},
		{source: "Help", targetFR: "Aide", targetDE: "Hilfe", targetJA: "ヘルプ"},
		{source: "Save", targetFR: "Enregistrer", targetDE: "Speichern", targetJA: "保存"},
		{source: "Save As...", targetFR: "Enregistrer sous...", targetDE: "Speichern unter...", targetJA: "名前を付けて保存..."},
		{source: "Open", targetFR: "Ouvrir", targetDE: "Öffnen", targetJA: "開く"},
		{source: "Close", targetFR: "Fermer", targetDE: "Schließen", targetJA: "閉じる"},
		{source: "Undo", targetFR: "Annuler", targetDE: "Rückgängig", targetJA: "元に戻す"},
		{source: "Redo", targetFR: "Rétablir", targetDE: "Wiederholen", targetJA: "やり直す"},
		{source: "Cut", targetFR: "Couper", targetDE: "Ausschneiden", targetJA: "切り取り"},
		{source: "Copy", targetFR: "Copier", targetDE: "Kopieren", targetJA: "コピー"},
		{source: "Paste", targetFR: "Coller", targetDE: "Einfügen", targetJA: "貼り付け"},
		{source: "Delete", targetFR: "Supprimer", targetDE: "Löschen", targetJA: "削除"},
		{source: "Select All", targetFR: "Tout sélectionner", targetDE: "Alles auswählen", targetJA: "すべて選択"},
		{source: "Find...", targetFR: "Rechercher...", targetDE: "Suchen...", targetJA: "検索..."},
		{source: "Find and Replace...", targetFR: "Rechercher et remplacer...", targetDE: "Suchen und Ersetzen...", targetJA: "検索と置換..."},
		{source: "Are you sure you want to delete this item?", targetFR: "Êtes-vous sûr de vouloir supprimer cet élément ?", targetDE: "Möchten Sie dieses Element wirklich löschen?", targetJA: "このアイテムを削除してもよろしいですか？"},
		{source: "Your changes have been saved successfully.", targetFR: "Vos modifications ont été enregistrées avec succès.", targetDE: "Ihre Änderungen wurden erfolgreich gespeichert.", targetJA: "変更が正常に保存されました。"},
		{source: "An unexpected error occurred. Please try again.", targetFR: "Une erreur inattendue s'est produite. Veuillez réessayer.", targetDE: "Ein unerwarteter Fehler ist aufgetreten. Bitte versuchen Sie es erneut.", targetJA: "予期しないエラーが発生しました。もう一度お試しください。"},
		{source: "New Task", targetFR: "Nouvelle tâche", targetDE: "Neue Aufgabe", targetJA: "新しいタスク"},
		{source: "Due Date", targetFR: "Date d'échéance", targetDE: "Fälligkeitsdatum", targetJA: "期限"},
		{source: "Priority", targetFR: "Priorité", targetDE: "Priorität", targetJA: "優先度"},
		{source: "Status", targetFR: "Statut", targetDE: "Status", targetJA: "ステータス"},
		{source: "Description", targetFR: "Description", targetDE: "Beschreibung", targetJA: "説明"},
		{source: "Comments", targetFR: "Commentaires", targetDE: "Kommentare", targetJA: "コメント"},
		{source: "Tags", targetFR: "Étiquettes", targetDE: "Tags", targetJA: "タグ"},
	}
	tmData := buildTMJSON(tmEntries)

	// Software UI terminology
	termsData := buildTermsJSON("Task Manager", []conceptDef{
		{domain: "ui", definition: "The name of the task management feature", terms: []termDef{
			{text: "task", locale: "en", status: "preferred"},
			{text: "tâche", locale: "fr", status: "preferred"},
			{text: "Aufgabe", locale: "de", status: "preferred"},
			{text: "タスク", locale: "ja", status: "preferred"},
		}},
		{domain: "ui", definition: "Person assigned to complete work", terms: []termDef{
			{text: "assignee", locale: "en", status: "preferred"},
			{text: "responsable", locale: "fr", status: "preferred"},
			{text: "Zuständiger", locale: "de", status: "preferred"},
			{text: "担当者", locale: "ja", status: "preferred"},
		}},
		{domain: "ui", definition: "Importance level of a work item", terms: []termDef{
			{text: "priority", locale: "en", status: "preferred"},
			{text: "priorité", locale: "fr", status: "preferred"},
			{text: "Priorität", locale: "de", status: "preferred"},
			{text: "優先度", locale: "ja", status: "preferred"},
		}},
		{domain: "ui", definition: "File added to a work item", terms: []termDef{
			{text: "attachment", locale: "en", status: "preferred"},
			{text: "pièce jointe", locale: "fr", status: "preferred"},
			{text: "Anhang", locale: "de", status: "preferred"},
			{text: "添付ファイル", locale: "ja", status: "preferred"},
		}},
		{domain: "brand", definition: "Product name - do not translate", terms: []termDef{
			{text: "Task Manager Pro", locale: "en", status: "preferred"},
			{text: "Task Manager Pro", locale: "fr", status: "preferred", note: "Ne pas traduire"},
			{text: "Task Manager Pro", locale: "de", status: "preferred", note: "Nicht übersetzen"},
			{text: "Task Manager Pro", locale: "ja", status: "preferred", note: "翻訳しない"},
		}},
	})

	return writeKAZ("apps/bowrain/samples/software-ui.kaz", kaz.PackOptions{
		Name:          "Task Manager Pro UI",
		SourceLocale:  "en",
		TargetLocales: []string{"fr", "de", "ja"},
		Items: []kaz.PackItem{
			{Name: "strings.json", Format: "json", Type: "file", SourceBytes: jsonSource, Parts: parts},
		},
		TMData:    tmData,
		TermsData: termsData,
	})
}

// generateMarketingProject creates a fully-translated marketing doc project.
func generateMarketingProject() error {
	fmt.Println("  Creating: marketing-content.kaz (fully translated, with TM)")

	htmlSource := []byte(`<html>
<body>
<h1>Introducing CloudSync Pro</h1>
<p>The next generation of cloud synchronization is here. CloudSync Pro automatically backs up your files, photos, and documents across all your devices.</p>
<h2>Key Features</h2>
<ul>
<li>Automatic file synchronization across unlimited devices</li>
<li>End-to-end encryption for maximum security</li>
<li>Smart conflict resolution with version history</li>
<li>Selective sync to save local storage space</li>
</ul>
<h2>Pricing</h2>
<p>Start your free 30-day trial today. No credit card required.</p>
<p>Personal plan: $9.99/month | Business plan: $24.99/month per user</p>
</body>
</html>`)

	parts := buildHTMLParts(htmlSource, []blockDef{
		{id: "h1", source: "Introducing CloudSync Pro", targetFR: "Présentation de CloudSync Pro", targetDE: "Vorstellung von CloudSync Pro", targetES: "Presentamos CloudSync Pro"},
		{id: "p1", source: "The next generation of cloud synchronization is here. CloudSync Pro automatically backs up your files, photos, and documents across all your devices.", targetFR: "La nouvelle génération de synchronisation cloud est arrivée. CloudSync Pro sauvegarde automatiquement vos fichiers, photos et documents sur tous vos appareils.", targetDE: "Die nächste Generation der Cloud-Synchronisation ist da. CloudSync Pro sichert automatisch Ihre Dateien, Fotos und Dokumente auf allen Ihren Geräten.", targetES: "La nueva generación de sincronización en la nube está aquí. CloudSync Pro respalda automáticamente sus archivos, fotos y documentos en todos sus dispositivos."},
		{id: "h2a", source: "Key Features", targetFR: "Fonctionnalités clés", targetDE: "Hauptfunktionen", targetES: "Características principales"},
		{id: "li1", source: "Automatic file synchronization across unlimited devices", targetFR: "Synchronisation automatique des fichiers sur un nombre illimité d'appareils", targetDE: "Automatische Dateisynchronisation über unbegrenzt viele Geräte", targetES: "Sincronización automática de archivos en dispositivos ilimitados"},
		{id: "li2", source: "End-to-end encryption for maximum security", targetFR: "Chiffrement de bout en bout pour une sécurité maximale", targetDE: "Ende-zu-Ende-Verschlüsselung für maximale Sicherheit", targetES: "Cifrado de extremo a extremo para máxima seguridad"},
		{id: "li3", source: "Smart conflict resolution with version history", targetFR: "Résolution intelligente des conflits avec historique des versions", targetDE: "Intelligente Konfliktlösung mit Versionshistorie", targetES: "Resolución inteligente de conflictos con historial de versiones"},
		{id: "li4", source: "Selective sync to save local storage space", targetFR: "Synchronisation sélective pour économiser l'espace de stockage local", targetDE: "Selektive Synchronisation zur Einsparung von lokalem Speicherplatz", targetES: "Sincronización selectiva para ahorrar espacio de almacenamiento local"},
		{id: "h2b", source: "Pricing", targetFR: "Tarifs", targetDE: "Preise", targetES: "Precios"},
		{id: "p2", source: "Start your free 30-day trial today. No credit card required.", targetFR: "Commencez votre essai gratuit de 30 jours dès aujourd'hui. Aucune carte de crédit requise.", targetDE: "Starten Sie noch heute Ihre kostenlose 30-Tage-Testversion. Keine Kreditkarte erforderlich.", targetES: "Comience su prueba gratuita de 30 días hoy. No se requiere tarjeta de crédito."},
		{id: "p3", source: "Personal plan: $9.99/month | Business plan: $24.99/month per user", nonTranslatable: true},
	})

	// TM from this translation
	tmEntries := []tmEntry{
		{source: "Introducing CloudSync Pro", targetFR: "Présentation de CloudSync Pro", targetDE: "Vorstellung von CloudSync Pro", targetES: "Presentamos CloudSync Pro"},
		{source: "Key Features", targetFR: "Fonctionnalités clés", targetDE: "Hauptfunktionen", targetES: "Características principales"},
		{source: "End-to-end encryption for maximum security", targetFR: "Chiffrement de bout en bout pour une sécurité maximale", targetDE: "Ende-zu-Ende-Verschlüsselung für maximale Sicherheit", targetES: "Cifrado de extremo a extremo para máxima seguridad"},
		{source: "Pricing", targetFR: "Tarifs", targetDE: "Preise", targetES: "Precios"},
		{source: "Start your free 30-day trial today.", targetFR: "Commencez votre essai gratuit de 30 jours dès aujourd'hui.", targetDE: "Starten Sie noch heute Ihre kostenlose 30-Tage-Testversion.", targetES: "Comience su prueba gratuita de 30 días hoy."},
		{source: "No credit card required.", targetFR: "Aucune carte de crédit requise.", targetDE: "Keine Kreditkarte erforderlich.", targetES: "No se requiere tarjeta de crédito."},
	}
	tmData := buildTMJSON(tmEntries)

	// Terms for brand consistency
	termsData := buildTermsJSON("CloudSync", []conceptDef{
		{domain: "product", definition: "Cloud sync product name", terms: []termDef{
			{text: "CloudSync Pro", locale: "en", status: "preferred"},
			{text: "CloudSync Pro", locale: "fr", status: "preferred", note: "Ne pas traduire le nom du produit"},
			{text: "CloudSync Pro", locale: "de", status: "preferred", note: "Produktname nicht übersetzen"},
			{text: "CloudSync Pro", locale: "es", status: "preferred", note: "No traducir el nombre del producto"},
		}},
		{domain: "security", definition: "Encryption that only sender and receiver can read", terms: []termDef{
			{text: "end-to-end encryption", locale: "en", status: "preferred"},
			{text: "chiffrement de bout en bout", locale: "fr", status: "preferred"},
			{text: "Ende-zu-Ende-Verschlüsselung", locale: "de", status: "preferred"},
			{text: "cifrado de extremo a extremo", locale: "es", status: "preferred"},
		}},
		{domain: "marketing", definition: "Period of free product usage", terms: []termDef{
			{text: "free trial", locale: "en", status: "preferred"},
			{text: "essai gratuit", locale: "fr", status: "preferred"},
			{text: "kostenlose Testversion", locale: "de", status: "preferred"},
			{text: "prueba gratuita", locale: "es", status: "preferred"},
		}},
	})

	return writeKAZ("apps/bowrain/samples/marketing-content.kaz", kaz.PackOptions{
		Name:          "CloudSync Marketing Page",
		SourceLocale:  "en",
		TargetLocales: []string{"fr", "de", "es"},
		Items: []kaz.PackItem{
			{Name: "landing-page.html", Format: "html", Type: "file", SourceBytes: htmlSource, Parts: parts},
		},
		TMData:    tmData,
		TermsData: termsData,
	})
}

// Helper types and functions

type blockDef struct {
	id             string
	source         string
	targetFR       string
	targetDE       string
	targetES       string
	targetJA       string
	translatable   bool // ignored — use nonTranslatable instead
	nonTranslatable bool // set to true to mark as non-translatable
}

type tmEntry struct {
	source   string
	targetFR string
	targetDE string
	targetES string
	targetJA string
}

type conceptDef struct {
	domain     string
	definition string
	terms      []termDef
}

type termDef struct {
	text   string
	locale string
	status string
	note   string
}

func buildHTMLParts(sourceBytes []byte, blocks []blockDef) []*model.Part {
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc", Name: "document", Format: "html"}},
	}

	for _, b := range blocks {
		block := model.NewBlock(b.id, b.source)
		block.Translatable = !b.nonTranslatable

		if b.targetFR != "" {
			block.SetTargetText("fr", b.targetFR)
		}
		if b.targetDE != "" {
			block.SetTargetText("de", b.targetDE)
		}
		if b.targetES != "" {
			block.SetTargetText("es", b.targetES)
		}
		if b.targetJA != "" {
			block.SetTargetText("ja", b.targetJA)
		}

		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: block})
	}

	return parts
}

func buildJSONParts(blocks []blockDef) []*model.Part {
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc", Name: "document", Format: "json"}},
	}

	for _, b := range blocks {
		block := model.NewBlock(b.id, b.source)
		block.Translatable = !b.nonTranslatable

		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: block})
	}

	return parts
}

type tmJSONEntry struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func buildTMJSON(entries []tmEntry) []byte {
	now := time.Now().UTC().Format(time.RFC3339)
	var jEntries []tmJSONEntry
	id := 1

	for _, e := range entries {
		if e.targetFR != "" {
			jEntries = append(jEntries, tmJSONEntry{
				ID: fmt.Sprintf("tm-%d", id), Source: e.source, Target: e.targetFR,
				SourceLocale: "en", TargetLocale: "fr", CreatedAt: now, UpdatedAt: now,
			})
			id++
		}
		if e.targetDE != "" {
			jEntries = append(jEntries, tmJSONEntry{
				ID: fmt.Sprintf("tm-%d", id), Source: e.source, Target: e.targetDE,
				SourceLocale: "en", TargetLocale: "de", CreatedAt: now, UpdatedAt: now,
			})
			id++
		}
		if e.targetES != "" {
			jEntries = append(jEntries, tmJSONEntry{
				ID: fmt.Sprintf("tm-%d", id), Source: e.source, Target: e.targetES,
				SourceLocale: "en", TargetLocale: "es", CreatedAt: now, UpdatedAt: now,
			})
			id++
		}
		if e.targetJA != "" {
			jEntries = append(jEntries, tmJSONEntry{
				ID: fmt.Sprintf("tm-%d", id), Source: e.source, Target: e.targetJA,
				SourceLocale: "en", TargetLocale: "ja", CreatedAt: now, UpdatedAt: now,
			})
			id++
		}
	}

	data, _ := json.Marshal(jEntries)
	return data
}

func buildTermsJSON(name string, concepts []conceptDef) []byte {
	doc := termbase.JSONTermBase{
		Name:    name,
		Version: "1.0",
	}

	for i, c := range concepts {
		concept := termbase.Concept{
			ID:         fmt.Sprintf("c%d", i+1),
			Domain:     c.domain,
			Definition: c.definition,
		}
		for _, t := range c.terms {
			term := termbase.Term{
				Text:   t.text,
				Locale: model.LocaleID(t.locale),
				Status: model.TermStatus(t.status),
				Note:   t.note,
			}
			concept.Terms = append(concept.Terms, term)
		}
		doc.Concepts = append(doc.Concepts, concept)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.Encode(doc)
	return buf.Bytes()
}

func writeKAZ(path string, opts kaz.PackOptions) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return kaz.Pack(f, opts)
}
