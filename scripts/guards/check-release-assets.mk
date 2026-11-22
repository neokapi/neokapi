GUARDS += check-release-assets
GUARD_LANES.check-release-assets := lint pre-push ci
check-release-assets: ## Guard: every release registers the platforms its workflow builds, and the asset checks work
	@python3 ./scripts/release-assets.py --self-test
	@python3 ./scripts/release-assets.py check-workflows
