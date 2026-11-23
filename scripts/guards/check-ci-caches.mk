GUARDS += check-ci-caches
GUARD_LANES.check-ci-caches := lint ci
check-ci-caches: ## Guard: only a run on main saves an Actions cache
	@python3 ./scripts/check-ci-caches.py --self-test
	@python3 ./scripts/check-ci-caches.py
	@python3 ./scripts/ci-cache-budget.py --self-test
