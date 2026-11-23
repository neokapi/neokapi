# The apt-install action's bounds: every limit positive, no attempt started on
# a spent budget, the whole install inside its budget.
GUARDS += check-apt-retry
GUARD_LANES.check-apt-retry := lint pre-push ci
GUARD_PATHS.check-apt-retry = ^\.github/actions/apt-install/
check-apt-retry: ## Guard: apt-install keeps every limit positive and the install inside its budget
	@bash ./.github/actions/apt-install/apt-retry.sh --self-test
