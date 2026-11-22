# Ungated: prose describing a project's context as a file is written
# wherever someone reaches for a path, and it teaches the habit the
# store-only model exists to end. About a second.
GUARDS += check-docs-store-only
GUARD_LANES.check-docs-store-only := lint pre-push ci
check-docs-store-only: ## Guard: the written record describes a project's context as living in its store
	@./scripts/check-docs-store-only.sh
