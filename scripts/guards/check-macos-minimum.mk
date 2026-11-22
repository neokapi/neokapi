GUARDS += check-macos-minimum
GUARD_LANES.check-macos-minimum := lint pre-push ci
check-macos-minimum: ## Guard: every declared macOS minimum is scripts/lib/macos-minimum, and Go builds for no newer
	@./scripts/check-macos-minimum.sh --self-test
	@./scripts/check-macos-minimum.sh
