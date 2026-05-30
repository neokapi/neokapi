package addin

import "fmt"

// This file models the subset of the google.apps.card.v1 JSON the Google
// Workspace add-on returns from its HTTP endpoints, plus the navigation/render
// envelopes. An HTTP (alternate-runtime) add-on returns raw Card JSON; there is
// no Apps Script CardService object involved.
//
// Reference: https://developers.google.com/workspace/add-ons/reference/rpc/google.apps.card.v1

// Card is a single add-on card (the sidebar contents).
type Card struct {
	Header   *CardHeader `json:"header,omitempty"`
	Sections []Section   `json:"sections,omitempty"`
}

// CardHeader is the card's title block.
type CardHeader struct {
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle,omitempty"`
	ImageURL  string `json:"imageUrl,omitempty"`
	ImageType string `json:"imageType,omitempty"`
}

// Section groups widgets under an optional header.
type Section struct {
	Header                    string   `json:"header,omitempty"`
	Collapsible               bool     `json:"collapsible,omitempty"`
	UncollapsibleWidgetsCount int      `json:"uncollapsibleWidgetsCount,omitempty"`
	Widgets                   []Widget `json:"widgets"`
}

// Widget is a tagged union; exactly one field is set.
type Widget struct {
	TextParagraph  *TextParagraph  `json:"textParagraph,omitempty"`
	DecoratedText  *DecoratedText  `json:"decoratedText,omitempty"`
	ButtonList     *ButtonList     `json:"buttonList,omitempty"`
	SelectionInput *SelectionInput `json:"selectionInput,omitempty"`
	ChipList       *ChipList       `json:"chipList,omitempty"`
	Divider        *struct{}       `json:"divider,omitempty"`
}

// TextParagraph renders a block of (optionally markdown) text.
type TextParagraph struct {
	Text string `json:"text"`
}

// DecoratedText is a rich text row — ideal for a finding with an icon and a fix.
type DecoratedText struct {
	StartIcon   *Icon   `json:"startIcon,omitempty"`
	TopLabel    string  `json:"topLabel,omitempty"`
	Text        string  `json:"text"`
	BottomLabel string  `json:"bottomLabel,omitempty"`
	WrapText    bool    `json:"wrapText,omitempty"`
	Button      *Button `json:"button,omitempty"`
}

// Icon is a material/known icon reference.
type Icon struct {
	MaterialIcon *MaterialIcon `json:"materialIcon,omitempty"`
	AltText      string        `json:"altText,omitempty"`
}

// MaterialIcon names a Google Material Symbols icon.
type MaterialIcon struct {
	Name string `json:"name"`
}

// Button is a clickable action.
type Button struct {
	Text    string   `json:"text"`
	Type    string   `json:"type,omitempty"` // OUTLINED|FILLED|FILLED_TONAL|BORDERLESS
	OnClick *OnClick `json:"onClick,omitempty"`
}

// ButtonList holds a row of buttons.
type ButtonList struct {
	Buttons []Button `json:"buttons"`
}

// SelectionInput is a dropdown / radio / checkbox group.
type SelectionInput struct {
	Name  string          `json:"name"`
	Label string          `json:"label,omitempty"`
	Type  string          `json:"type"` // DROPDOWN|RADIO_BUTTON|CHECK_BOX
	Items []SelectionItem `json:"items"`
}

// SelectionItem is one option in a SelectionInput.
type SelectionItem struct {
	Text     string `json:"text"`
	Value    string `json:"value"`
	Selected bool   `json:"selected,omitempty"`
}

// ChipList is a wrapped list of chips (used for the approved-terms glossary).
type ChipList struct {
	Layout string `json:"layout,omitempty"` // WRAPPED|HORIZONTAL_SCROLLABLE
	Chips  []Chip `json:"chips"`
}

// Chip is a single labelled chip.
type Chip struct {
	Label string `json:"label"`
}

// OnClick wraps an Action (server callback) or an OpenLink.
type OnClick struct {
	Action *Action `json:"action,omitempty"`
}

// Action is a callback to one of the add-on's HTTP endpoints.
type Action struct {
	Function      string            `json:"function"`
	Parameters    []ActionParameter `json:"parameters,omitempty"`
	LoadIndicator string            `json:"loadIndicator,omitempty"` // SPINNER|NONE
	PersistValues bool              `json:"persistValues,omitempty"`
}

// ActionParameter is a key/value passed back to the endpoint on click.
type ActionParameter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ---------------------------------------------------------------------------
// Render envelopes
// ---------------------------------------------------------------------------

// homepageResponse is the envelope a homepage / file-scope-granted trigger
// returns: it pushes the initial card.
type homepageResponse struct {
	Action navAction `json:"action"`
}

// clickResponse is the envelope a button click handler returns: it updates the
// current card in place and may show a transient notification.
type clickResponse struct {
	RenderActions renderActions `json:"renderActions"`
}

type renderActions struct {
	Action navAction `json:"action"`
}

type navAction struct {
	Navigations  []navigation  `json:"navigations,omitempty"`
	Notification *notification `json:"notification,omitempty"`
}

type navigation struct {
	PushCard   *Card `json:"pushCard,omitempty"`
	UpdateCard *Card `json:"updateCard,omitempty"`
}

type notification struct {
	Text string `json:"text"`
}

// fileScopeResponse asks Google to prompt the user to grant the add-on
// per-file (drive.file) access to the active document. The exact key casing for
// the HTTP runtime mirrors the Apps Script EditorFileScopeActionResponse
// (requestFileScopeForActiveDocument).
type fileScopeResponse struct {
	RenderActions fileScopeRenderActions `json:"renderActions"`
}

type fileScopeRenderActions struct {
	HostAppAction hostAppAction `json:"hostAppAction"`
}

type hostAppAction struct {
	EditorAction editorAction `json:"editorAction"`
}

type editorAction struct {
	RequestFileScopeForActiveDocument struct{} `json:"requestFileScopeForActiveDocument"`
}

// ---------------------------------------------------------------------------
// Card builders
// ---------------------------------------------------------------------------

// pushHomepage wraps a card in the homepage push envelope.
func pushHomepage(card *Card) homepageResponse {
	return homepageResponse{Action: navAction{Navigations: []navigation{{PushCard: card}}}}
}

// updateCard wraps a card (and optional toast) in the click-update envelope.
func updateCard(card *Card, toast string) clickResponse {
	act := navAction{Navigations: []navigation{{UpdateCard: card}}}
	if toast != "" {
		act.Notification = &notification{Text: toast}
	}
	return clickResponse{RenderActions: renderActions{Action: act}}
}

// requestFileScope builds the file-scope prompt response.
func requestFileScope() fileScopeResponse {
	return fileScopeResponse{}
}

// grantAccessCard is the card shown before the user grants per-file access.
func grantAccessCard(endpoint string) *Card {
	return &Card{
		Header: &CardHeader{Title: "Bowrain", Subtitle: "Brand, terminology & translation"},
		Sections: []Section{{
			Widgets: []Widget{
				{TextParagraph: &TextParagraph{Text: "Grant Bowrain access to this document to check it against your brand voice, look up approved terminology, and translate it on-brand."}},
				{ButtonList: &ButtonList{Buttons: []Button{{
					Text: "Grant access",
					Type: "FILLED",
					OnClick: &OnClick{Action: &Action{
						Function:      endpoint + "/google/authorize",
						LoadIndicator: "SPINNER",
					}},
				}}}},
			},
		}},
	}
}

// homeCard is the main sidebar: brand-voice findings, a terminology glossary, a
// target-language picker, and Scan / Translate actions.
func homeCard(endpoint, title string, check *CheckResult, terms *TermsResult, targets []SelectionItem) *Card {
	card := &Card{Header: &CardHeader{Title: "Bowrain", Subtitle: title}}

	// Brand-voice findings.
	findingsSection := Section{Header: "Brand voice"}
	if check != nil && len(check.Findings) > 0 {
		findingsSection.Header = fmt.Sprintf("Brand voice · score %d", check.Score)
		for _, f := range check.Findings {
			dt := &DecoratedText{
				StartIcon: &Icon{MaterialIcon: &MaterialIcon{Name: severityIcon(f.Severity)}, AltText: f.Severity},
				TopLabel:  fmt.Sprintf("%s · %s", f.Category, f.Severity),
				Text:      f.Message,
				WrapText:  true,
			}
			if f.Suggestion != "" {
				dt.BottomLabel = "Suggest: " + f.Suggestion
			}
			findingsSection.Widgets = append(findingsSection.Widgets, Widget{DecoratedText: dt})
		}
	} else if check != nil {
		findingsSection.Header = fmt.Sprintf("Brand voice · score %d", check.Score)
		findingsSection.Widgets = append(findingsSection.Widgets, Widget{
			TextParagraph: &TextParagraph{Text: "No brand-voice issues found. ✔"},
		})
	} else {
		findingsSection.Widgets = append(findingsSection.Widgets, Widget{
			TextParagraph: &TextParagraph{Text: "Run a scan to check this document against your brand voice."},
		})
	}
	card.Sections = append(card.Sections, findingsSection)

	// Terminology glossary.
	if terms != nil && len(terms.Matches) > 0 {
		chips := make([]Chip, 0, len(terms.Matches))
		for _, m := range terms.Matches {
			chips = append(chips, Chip{Label: m.Term})
		}
		card.Sections = append(card.Sections, Section{
			Header:  "Terminology in this document",
			Widgets: []Widget{{ChipList: &ChipList{Layout: "WRAPPED", Chips: chips}}},
		})
	}

	// Translate.
	card.Sections = append(card.Sections, Section{
		Header: "Translate",
		Widgets: []Widget{
			{SelectionInput: &SelectionInput{
				Name:  "targetLang",
				Label: "Target language",
				Type:  "DROPDOWN",
				Items: targets,
			}},
			{ButtonList: &ButtonList{Buttons: []Button{
				{Text: "Scan", Type: "OUTLINED", OnClick: &OnClick{Action: &Action{
					Function: endpoint + "/google/scan", LoadIndicator: "SPINNER",
				}}},
				{Text: "Translate", Type: "FILLED", OnClick: &OnClick{Action: &Action{
					Function: endpoint + "/google/translate", LoadIndicator: "SPINNER", PersistValues: true,
				}}},
			}}},
		},
	})
	return card
}

// severityIcon maps a finding severity to a Material Symbols icon name.
func severityIcon(severity string) string {
	switch severity {
	case "critical", "major":
		return "error"
	case "minor":
		return "warning"
	default:
		return "info"
	}
}

// defaultTargetItems is the language picker shown when the project does not
// constrain target locales.
func defaultTargetItems(selected string) []SelectionItem {
	langs := []struct{ label, value string }{
		{"French (fr)", "fr"},
		{"German (de)", "de"},
		{"Spanish (es)", "es"},
		{"Japanese (ja)", "ja"},
		{"Portuguese (pt)", "pt"},
	}
	items := make([]SelectionItem, 0, len(langs))
	for i, l := range langs {
		items = append(items, SelectionItem{Text: l.label, Value: l.value, Selected: (selected == "" && i == 0) || selected == l.value})
	}
	return items
}
