# `CI passed` is the one status check main requires. A job missing from its
# `needs` would not hold a merge. See .claude/skills/verify/ci.md.
GUARDS += check-ci-passed
GUARD_LANES.check-ci-passed := lint pre-push ci
check-ci-passed: ## Guard: the CI passed job lists every job of the CI workflow
	@python3 ./scripts/ci-passed.py --self-test
	@python3 ./scripts/ci-passed.py needs
