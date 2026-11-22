# Every consumer of the registry enumerates scripts/guards/, so a guard file
# that make does not load, or a guard wired straight into `lint:`, pre-push or
# the Repo guards job, would run in some places and not others with nothing to
# say so. This proves the checks on fixtures, then checks this repository.
GUARDS += check-guard-registry
GUARD_LANES.check-guard-registry := lint pre-push ci
check-guard-registry: ## Guard: every guard is registered once, in scripts/guards/, and wired nowhere else
	@python3 ./scripts/check-guard-registry.py --self-test
	@python3 ./scripts/check-guard-registry.py
