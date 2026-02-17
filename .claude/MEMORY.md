# gokapi Project Memory

## ADR Conventions
- Files named `NNN-topic.md` (no `adr-` prefix) in `docs/adr/`
- Each has frontmatter: id, sidebar_position, title
- Structure: Context → Decision → Alternatives Considered → Consequences
- ADRs describe current state, updated in place (not appended chronologically)
- README.md has an index table

## Key Architecture Points
- Content flows as `Part` (with `PartType` discriminator + `Resource` interface) through channel-based pipeline
- `Block` is the translatable unit; carries `Properties` (map[string]string) and `Annotations` (map[string]Annotation)
- Tools embed `BaseTool` and set handler function fields; unhandled Part types pass through
- Sievepen = built-in TM library (in-memory + SQLite backends)
- Bowrain = Wails v3 desktop app (Go backend + React 19 frontend)
- KAZ = ZIP-based project archive format

## ADR-010: Content-Aware TM (Sievepen)
- TMEntry stores `*model.Fragment` (not strings) — preserves inline Spans and entity metadata
- Derived matching keys: plain (Fragment.Text()), structural ({1},{2}), generalized ({PERSON},{PRODUCT})
- Tiered matching: generalized-exact → structural-exact → plain-exact → generalized/structural/plain-fuzzy
- EntityMapping links source+target entity positions; EntityAdaptation maps stored→current values
- Lookup takes `*model.Block` (not string) — reads entity annotations for generalized keys
- `entity-annotate` tool (ADR-016) is the single source of entity info for TM generalization
- Privacy redaction is orthogonal — TM handles generalization natively

## ADR-016: Terminology & Brand Management
- Progressive complexity: CSV glossary → concept-oriented termbase → streams → brand governance
- Concept-oriented data model (TBX-inspired) with `Concept` containing `Term` entries per locale
- TermBase interface mirrors Sievepen pattern (in-memory + SQLite backends)
- Shared SQLite infra layer in `bowrain/storage/` used by both TermBase and Sievepen
- Six pipeline tools: term-lookup, term-enforce, term-extract (AI), entity-annotate (AI), redact, unredact
- entity-annotate feeds both TM generalization (ADR-010) and terminology management
- redact/unredact are privacy-only tools, orthogonal to TM generalization
- Two new annotation types: TermAnnotation, EntityAnnotation — both with character-level TextRange positions
- KAZ embeds read-only termbase snapshot; master lives externally
- Tiered matching: exact → normalized → fuzzy (default), stemming + AI opt-in
- Terminology Streams: named what-if experiments with side-by-side preview and atomic promotion
- Streams have optional target dates (informational only, manual promotion required)
- Inline term suggestion during translation feeds same lifecycle as terminology module
- Real-time always-on term highlighting in Bowrain (Aho-Corasick index)
- Flat context tags in Phase 1; first-class entities deferred to Phase 2/3 based on usage
- Brand voice/tone rules planned for Phase 3 as pipeline tool
