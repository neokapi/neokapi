// Package mdspike is a SPIKE. It is not wired into any command, any flow, or
// any host service, and nothing in the repository imports it outside its own
// tests.
//
// It exists to answer one question with running code: should a voice profile be
// authored as YAML (today's [profile.LoadProfileYAML]) or as markdown with
// frontmatter, in the shape of an agent skill file?
//
// The form it implements splits the document along the line the profile itself
// already draws:
//
//   - Frontmatter carries the governing structure — tone enums, style booleans
//     and limits, regex prohibitions with their severities, vocabulary rules.
//     These decide whether content passes or fails, so they are authored and
//     reviewed, never regenerated.
//   - The body carries the prose — description, tone guidelines, before/after
//     examples, per-locale cultural notes. This is the material that actually
//     reaches the model through [profile.RenderVoiceGuide], and the material a
//     non-engineer reads.
//
// On top of that split it demonstrates two things the YAML form cannot express:
//
//   - Inheritance. A profile may `extends:` another, so house rules are declared
//     once instead of copied between profiles (see the drift already visible
//     between context/brand-voice.yaml and context/bowrain-voice.yaml).
//   - Terms-store sourcing. A profile may derive vocabulary rules from the terms
//     store by status and domain instead of restating them, so there is one
//     place where "deprecated" is decided.
//
// The findings, and the recommendation, are in
// docs/internals/spike-profile-authoring-format.md.
package mdspike
