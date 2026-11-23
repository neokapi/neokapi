# The ratchet itself runs on extraction in l10n.yml. This proves its
# comparison on planted warnings. Well under a second.
GUARDS += check-extract-warnings
GUARD_LANES.check-extract-warnings := pre-push ci
check-extract-warnings: ## Guard: the extractor warning ratchet refuses a new warning
	@node scripts/check-extract-warnings.mjs --self-test
