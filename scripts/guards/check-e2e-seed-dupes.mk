# A copied seed document can only land under one of the two e2e trees,
# so pre-push runs this only when one of them changed.
GUARDS += check-e2e-seed-dupes
GUARD_LANES.check-e2e-seed-dupes := lint pre-push ci
GUARD_PATHS.check-e2e-seed-dupes = ^bowrain/e2e/ ^bowrain/apps/web/e2e/
check-e2e-seed-dupes: ## Guard: the e2e seed documents live once, in bowrain/e2e/shared/seed-files
	@./scripts/check-e2e-seed-dupes.sh
