# A workflow that follows another one (`workflow_run`) runs with its own
# permissions, whoever started the run it follows, so each of its jobs must be
# held to runs of this repository. Triggers any GitHub account can cause are
# refused outright. See scripts/check-workflow-run-origin.py.
GUARDS += check-workflow-run-origin
GUARD_LANES.check-workflow-run-origin := lint pre-push ci
check-workflow-run-origin: ## Guard: no workflow acts for an account outside this repository
	@python3 ./scripts/check-workflow-run-origin.py --self-test
	@python3 ./scripts/check-workflow-run-origin.py
