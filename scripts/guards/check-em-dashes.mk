# Several tiers carry the same prose to a reader: Go string literals, the
# extracted source catalogs, the UI source no catalog reaches, the reference
# dossiers, and the Markdown docs. CLAUDE.md's ceiling applied to the docs
# alone, so the rest drifted into the thousands. Every part fails on a finding.
# The catalogs part reports (rather than fails on) a surface `make l10n-extract`
# has not produced yet, so this target is useful before a full extract; the
# l10n workflow runs it with --require-extracted, where the extract has just
# run.
GUARDS += check-em-dashes
GUARD_LANES.check-em-dashes := lint pre-push ci
check-em-dashes: ## Guard: no em dash in Go string literals, extracted source catalogs, unextracted UI source, the reference prose, or docs prose
	@./scripts/check-em-dashes.sh
