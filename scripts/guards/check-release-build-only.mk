# A release workflow that can be dispatched is a sharp tool: a build-only run
# must reach no step that publishes, signs or saves a cache.
GUARDS += check-release-build-only
GUARD_LANES.check-release-build-only := lint pre-push ci
GUARD_PATHS.check-release-build-only = ^\.github/workflows/(release(-bowrain|-check|-sat)?|rebuild-release-images)\.yml$$ ^scripts/check-release-build-only\.py$$ ^scripts/check-test-scripts\.py$$
check-release-build-only: ## Guard: a build-only run of a release workflow publishes, signs and saves nothing
	@python3 ./scripts/check-release-build-only.py --self-test
	@python3 ./scripts/check-release-build-only.py
