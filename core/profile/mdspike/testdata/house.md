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
vocabulary:
  forbidden_terms:
    - term: simply
      replacement: ""
      severity: minor
    - term: easily
      replacement: ""
      severity: minor
    - term: magic
      replacement: ""
      severity: major
---

# neokapi house rules

The prohibitions every neokapi and Bowrain profile inherits. Today these lines
are copied verbatim into both .kapi/voice.yaml and
.kapi/profiles/bowrain/voice.yaml; here they are declared once and inherited through
`extends:`.

Nothing brand-specific belongs in this file. A profile that extends it adds its
own tone, its own examples, and whatever further prohibitions its surface
needs — it cannot remove one of these.
