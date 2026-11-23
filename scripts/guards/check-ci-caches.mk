GUARDS += check-ci-caches
GUARD_LANES.check-ci-caches := lint ci
check-ci-caches: ## Guard: no step a pull request runs saves an Actions cache
	@python3 ./scripts/check-ci-caches.py --self-test
	@python3 ./scripts/check-ci-caches.py
	@python3 ./scripts/ci-cache-budget.py --self-test
