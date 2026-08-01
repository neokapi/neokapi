---
extends: house.md
name: restates a house rule
style:
  prohibited_patterns:
    - regex: '\b(powerful|seamless(?:ly)?|effortless(?:ly)?|blazing|game-changing|cutting-edge|revolutionary|supercharged?|unleash)\b'
      description: Marketing superlatives and hype words
      severity: critical
vocabulary:
  forbidden_terms:
    - term: simply
      replacement: ""
      severity: major
---

# Restates a house rule

A profile that repeats an inherited prohibition to tighten its severity. The
merge must replace the rule in place, not append a second copy: two copies
raise two findings for one violation.
