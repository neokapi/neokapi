package screens

// RulesDecideViewDTO is every question Decide asks about one project.
type RulesDecideViewDTO struct {
	// Project is the project's workspace key, and ProjectName what a person
	// calls it.
	Project     string            `json:"project"`
	ProjectName string            `json:"project_name"`
	Decisions   []RuleDecisionDTO `json:"decisions"`
	// Rules, Suggested and Narrowed are the counts `kapi rules list` opens
	// with: the rules in force, the suggestions waiting and the rules that
	// hold in only part of the project.
	Rules     int `json:"rules"`
	Suggested int `json:"suggested"`
	Narrowed  int `json:"narrowed"`
}

// RuleDecisionDTO is one question.
type RuleDecisionDTO struct {
	// ID names the suggestion; a disagreement has none, and each option
	// carries its side's.
	ID    string `json:"id,omitempty"`
	Short string `json:"short,omitempty"`
	// Kind is "name", "word", "note", "memory" or "disagreement".
	Kind     string `json:"kind"`
	Question string `json:"question"`
	// Reason says what a disagreement is about.
	Reason   string `json:"reason,omitempty"`
	Sentence string `json:"sentence"`
	// Label is the wording the question is about, for its row in the list,
	// and Avoided how many wordings a word rule's check reports.
	Label   string `json:"label"`
	Avoided int    `json:"avoided"`
	// Options are the forms of a name or the sides of a disagreement, the
	// most used first. Chosen is the option a keep takes unless the person
	// picks another.
	Options []RuleOptionDTO `json:"options"`
	Chosen  int             `json:"chosen"`
	// Other reports a question that takes a form the person types: a name.
	Other bool `json:"other"`
	// Consequence says what keeping it does, for a question with no options.
	Consequence string `json:"consequence,omitempty"`
	// Seen is the sentence the suggestion was seen in, and its file.
	Seen *ContextEvidenceDTO `json:"seen,omitempty"`
	// Applies is where a kept rule holds.
	Applies   string          `json:"applies"`
	NoticedBy ContextActorDTO `json:"noticed_by"`
}

// RuleOptionDTO is one answer to a question.
type RuleOptionDTO struct {
	// Form is one wording of a name; ID, Short and Sentence name and state one
	// side of a disagreement.
	Form     string `json:"form,omitempty"`
	ID       string `json:"id,omitempty"`
	Short    string `json:"short,omitempty"`
	Sentence string `json:"sentence,omitempty"`
	// Blocks and Files count where the project's source writes it.
	Blocks int `json:"blocks"`
	Files  int `json:"files"`
	// Consequence says what keeping this option does.
	Consequence string `json:"consequence"`
	// NoticedBy is who suggested the side, for a disagreement.
	NoticedBy *ContextActorDTO `json:"noticed_by,omitempty"`
}

// RuleDecideResultDTO is what one answer recorded.
type RuleDecideResultDTO struct {
	// Answer is "keep", "drop" or "later".
	Answer string `json:"answer"`
	// Kept and Dropped count the operations the answer recorded; a choice
	// between two sides keeps one and drops the rest.
	Kept    int `json:"kept"`
	Dropped int `json:"dropped"`
	// Later counts the suggestions left waiting, which records nothing.
	Later int `json:"later"`
}
