# The Dependabot relock keeps the update, lowers nothing else, keeps the base
# lockfile's integrity hashes, and fetches the base without changing the
# repository's depth.
GUARDS += check-dependabot-relock
GUARD_LANES.check-dependabot-relock := lint pre-push ci
GUARD_PATHS.check-dependabot-relock = ^scripts/dependabot-relock\.py$$ ^scripts/lib/pnpm_lockfile\.py$$ ^\.github/workflows/dependabot-pnpm-relock\.yml$$
check-dependabot-relock: ## Guard: the Dependabot relock keeps the update and lowers nothing else
	@python3 ./scripts/dependabot-relock.py --self-test
