# The gate itself runs only in the nightly convergence, where nothing
# exercises it on a pull request. Its self-test does: it drives the real
# classification against real scratch repositories, so a change to the
# derived set or to the classification cannot pass here and disarm the
# nightly there. About a second.
GUARDS += check-test-shards
GUARD_LANES.check-test-shards := pre-push ci
check-test-shards: ## Guard: the test shards of a package cover every test once
	@./scripts/go-test-shard.sh --self-test
