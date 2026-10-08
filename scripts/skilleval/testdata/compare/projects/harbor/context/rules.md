# Harbor writing guide

Follow this guide for everything you write in this repository: the
documentation (`docs/`), support replies (`support/`) and the legal terms
(`legal/`).

## Voice

Harbor writes for developers in the middle of a task. Tell them what to run and
what happens, precisely, and get out of the way.

- Precise, plain and direct. Neutral register and tone, no humor.
- Write instructions in the imperative and address the reader as "you".
  Present tense, active voice, no contractions.
- Put commands, flags, paths and config keys in backticks.
- State what Harbor does and under which conditions; never make an
  unqualified claim about uptime or security.
- Do not call a step simple or easy.
- Do not mention other products by name.

## Avoid

- "zero downtime" or "zero-downtime": say what Harbor does instead (traffic
  moves after the health check passes).
- Unqualified security or availability claims: "secure by default",
  "bulletproof", "unbreakable", "100% uptime".
- "simply", "just", "easily", "obviously".
- Exclamation marks.

## Words

| Write | Not |
| --- | --- |
| other deployment tools | Dockyard (a competitor; never name it) |
| allowlist, allowlisting | whitelist, whitelisting |
| denylist | blacklist |

## The Harbor Cloud rename

Harbor Cloud was renamed **Harbor Hosted**. The documentation (`docs/`) and
support replies (`support/`) use the new name. The legal terms (`legal/`) keep
"Harbor Cloud", the name the signed contracts use, until they are renewed.
