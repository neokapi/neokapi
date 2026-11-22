# It expands the extract globs the way the extractor does, so it needs
# `fs/promises.glob` from Node 22. No dependencies: it reads the flag sets
# from `make l10n-extract-globs` and globs the working tree.
GUARDS += check-extract-fixtures
GUARD_LANES.check-extract-fixtures := lint ci
check-extract-fixtures: ## Guard: no test/story file is extracted, and each surface's make and package invocations agree
	@node scripts/check-extract-fixtures.mjs
