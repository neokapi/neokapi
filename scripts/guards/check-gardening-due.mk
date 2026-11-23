# The gardening report must keep its rules: a missing source or a bad date is
# reported as due, never skipped. This proves them on fixtures; it never fails
# on what is due.
GUARDS += check-gardening-due
GUARD_LANES.check-gardening-due := lint pre-push ci
GUARD_PATHS.check-gardening-due = ^scripts/gardening-due\.py$$ ^docs/internals/gardening\.yaml$$
check-gardening-due: ## Guard: the gardening report's rules hold on fixtures, and the list reads
	@python3 ./scripts/gardening-due.py --self-test
	@python3 ./scripts/gardening-due.py --json > /dev/null
