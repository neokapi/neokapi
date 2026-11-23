# The sourcecode release takes its version from the tag and copies the
# manifest from the tagged commit, and only the tag push compares them.
# The self-test runs that comparison on matching and mismatched pairs, and
# checks that the release workflow still calls it.
GUARDS += check-release-manifest-version
GUARD_LANES.check-release-manifest-version := ci
check-release-manifest-version: ## Guard: a plugin release tag matches its manifest version
	@./scripts/check-release-manifest-version.sh --self-test
