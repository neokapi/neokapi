# A workflow that follows another one (`workflow_run`) runs with its own
# permissions, whoever started the run it follows. Each of its jobs must be
# held to runs of this repository. See scripts/check-workflow-run-origin.py.
GUARDS += check-workflow-run-origin
GUARD_LANES.check-workflow-run-origin := lint pre-push ci
check-workflow-run-origin: ## Guard: a workflow that follows another acts only for this repository's runs
	@python3 ./scripts/check-workflow-run-origin.py --self-test
	@python3 ./scripts/check-workflow-run-origin.py
