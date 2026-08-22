---
name: neokapi house rules
style:
  active_voice: true
  sentence_length: short
  person_pov: second
  contractions: sometimes
  prohibited_patterns:
    - regex: '\b(powerful|seamless(?:ly)?|effortless(?:ly)?|blazing|game-changing|cutting-edge|revolutionary|supercharged?|unleash)\b'
      description: Marketing superlatives and hype words
      severity: major
    - regex: '\b(production-proven|everything you need|localize at scale|just point and go)\b'
      description: Brochure framing
      severity: major
    - regex: '[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]'
      description: Emoji in committed prose
      severity: major
    - regex: '(^|[^\w.-])\d+\+?\s+(?:built-in\s+)?(formats|tools|providers|filters|languages)\b'
      description: Hardcoded counts that the code controls — name categories and link to the generated reference instead
      severity: critical
    - regex: '(?i)\b(?:magical(?:ly)?|magic\s+happens)\b|(?i)\b(?:(?:just\s+)?(?:like|pure|sheer)\s+magic|(?:it|that|this)(?:[''’]s|\s+is)\s+magic)\b(?:\s*[^\p{L}\p{N}\s_-]|$)'
      description: '"magic" as a claim about the product — state the mechanism; the technical sense (magic bytes, magic number) is not this rule'
      severity: major
    - regex: '\btermbases?\b'
      description: "termbase: the store is the terms store, and its contents are terms (TBX, the ISO standard, keeps its name)"
      severity: major
    - regex: '\bglossar(y|ies)\b'
      description: "glossary: the store is the terms store (XLIFF 2.x's Glossary module keeps its name)"
      severity: major
    - regex: '(?i)\blocali[sz](e|es|ed|ing|ation|ations)\b'
      description: Say multilingual content, or language — and recast the sentence rather than substituting a word for "localize"
      severity: major
    - regex: '(?i)(^|[^\w-])l10n\b'
      description: Say multilingual content, or language (the l10n-* make targets are developer-facing internals and are not this rule)
      severity: major
    - regex: '(?i)\btranslation memor(y|ies)\b'
      description: "translation memory: say content memory"
      severity: major
    - regex: '(?i)\bbrand[ -]voice\b'
      description: Voice is the mechanism — say voice profile. "Brand voice" belongs in prose about the common use case, never as the name of the thing
      severity: major
vocabulary:
  forbidden_terms:
    - term: simply
      replacement: ""
      severity: minor
    - term: easily
      replacement: ""
      severity: minor
---

# neokapi house rules

The prohibitions every neokapi and Bowrain profile inherits. Today these lines
are copied verbatim into both .kapi/voice.yaml and
.kapi/profiles/bowrain/voice.yaml; here they are declared once and inherited through
`extends:`.

Nothing brand-specific belongs in this file. A profile that extends it adds its
own tone, its own examples, and whatever further prohibitions its surface
needs — it cannot remove one of these.
