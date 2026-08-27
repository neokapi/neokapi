package main

// What the lab writes about, and what governs it at each point.
//
// The product is synthesized. A real one would let a model answer from what it
// already knows about that product rather than from what it was given, and the
// whole question here is what the given context does.
//
// Two documents, one subject. A user guide and a developer reference for the
// same feature is the ordinary case where the reader changes and nothing else
// does, so it is the case where a coordinate has something to do.

// productBrief is the same for every cell of the matrix. It states facts and
// says nothing about voice, register or audience: those are what the arms and
// the coordinates supply, and a brief that leaked them would answer the
// question before the model did.
const productBrief = `Harbourlight is a port logistics tool. Port operators use it to track
vessels, file cargo paperwork with customs, and manage berth bookings.

The feature to document is ARRIVAL ALERTS.

Facts:
- An arrival alert tells someone that a vessel has reported a new position or a
  revised arrival time.
- Alerts can go to email, to SMS, or to another system.
- A person chooses which vessels they want alerts for, and how close to arrival
  the alert should fire (any of 24 hours, 6 hours, 1 hour, or on berth).
- Alerts to another system are delivered as an HTTP POST to a URL the customer
  supplies. The body is JSON. It carries the vessel id, the alert type, the new
  estimated arrival time in UTC, and a signature.
- The signature is HMAC-SHA256 over the raw body, using a shared secret, sent in
  the X-Harbourlight-Signature header.
- Delivery is retried 5 times with exponential backoff. A delivery is considered
  failed after that and is visible in the alert history for 30 days.
- Quiet hours can be set per person; alerts that fall inside them are held and
  sent when quiet hours end, except "on berth" alerts, which always go out
  immediately.`

// Point is one coordinate in the context space, and what is bound there.
type Point struct {
	// Audience is the declared axis value. The coordinate map is open (only
	// product and channel are structural), so a project names the dimensions
	// its content actually varies along, and this one varies by reader.
	Audience string
	// Label is how the page names it.
	Label string
	// Task is the deliverable, stated in BOTH arms. The bare arm has to know it
	// is writing a user guide, or the comparison measures whether the model was
	// told who the reader is rather than what the governance did.
	Task string
	// Voice is the profile bound at this point. Both share the Harbourlight
	// brand voice; they differ in what the reader is assumed to know, which is
	// the thing the coordinate exists to carry.
	Voice string
}

// The Harbourlight brand voice, shared by both points. Held here as a fragment
// rather than a whole profile so the two points cannot drift apart on the
// dimensions that are supposed to be identical.
const brandVoice = `tone:
    personality:
        - plain
        - direct
        - calm
    formality: neutral
    emotion: neutral
    humor: none
    guidelines: Address the reader as you. State what to do next. No superlatives, no hedging.
style:
    active_voice: true
    person_pov: second
    contractions: sometimes
`

// points are the two coordinates the lab writes at.
//
// What differs between them is deliberately narrow: the same brand voice, the
// same product, the same feature, and a different assumption about what the
// reader already knows. If a coordinate does nothing, the two governed
// documents will read alike, and that is the result.
var points = []Point{
	{
		Audience: "end-user",
		Label:    "End user (non-technical)",
		Task: "Write the user guide for arrival alerts in Harbourlight, as a Markdown document " +
			"of roughly 600 words. The reader is a port operator doing their job, not a programmer.",
		// The jargon ban is expressed as forbidden_terms rather than as one
		// prohibited_pattern over an alternation, because the compact guide
		// renders a pattern by its description alone: the model would be told
		// "avoid implementation vocabulary" and none of the words it means
		// (#2240). Terms render as the words themselves, so this is the
		// mechanism that reaches a model today.
		Voice: `name: Harbourlight — end user
description: Harbourlight's voice for people using the product, who are not programmers
` + brandVoice + `    sentence_length: short
vocabulary:
    forbidden_terms:
        - term: endpoint
          replacement: address
          severity: major
          forms: [endpoints]
        - term: payload
          replacement: the alert's contents
          severity: major
          forms: [payloads]
        - term: webhook
          replacement: another system
          severity: major
          forms: [webhooks]
        - term: JSON
          replacement: a structured message
          severity: major
        - term: HMAC
          replacement: a signature
          severity: major
        - term: HTTP POST
          replacement: sends the alert
          severity: major
        - term: API
          replacement: another system
          severity: major
        - term: exponential backoff
          replacement: waits longer between tries
          severity: major
        - term: utilize
          severity: major
          forms: [utilizes, utilized, utilizing]
        - term: configure
          replacement: set up
          severity: minor
          forms: [configures, configured, configuring, configuration]
`,
	},
	{
		Audience: "developer",
		Label:    "Developer",
		Task: "Write the developer documentation for arrival alerts in Harbourlight, as a Markdown " +
			"document of roughly 600 words. The reader is integrating another system with it.",
		// The mirror image: this reader is assumed to have the vocabulary the
		// other is not, so what is banned here is the language that wastes
		// their time. Same reason for using terms over patterns (#2240).
		Voice: `name: Harbourlight — developer
description: Harbourlight's voice for developers integrating with the product
` + brandVoice + `    sentence_length: medium
vocabulary:
    forbidden_terms:
        - term: simply
          severity: major
        - term: just
          severity: major
        - term: easy
          replacement: say what it takes instead
          severity: major
          forms: [easily]
        - term: straightforward
          severity: major
        - term: utilize
          severity: major
          forms: [utilizes, utilized, utilizing]
        - term: seamless
          severity: major
          forms: [seamlessly]
`,
	},
}

// labModels is the matrix's model axis.
//
// Ids rather than aliases, and the run records what actually answered
// (modelUsage from the CLI) rather than what was asked, so a silent alias
// resolution cannot be read as a comparison between two models that were in
// fact one.
var labModels = []string{
	"claude-opus-5",
	"claude-sonnet-5",
	"claude-opus-4-8",
	"claude-haiku-4-5",
}
