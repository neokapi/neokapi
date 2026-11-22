# A package `test` script that no pull request job runs is a suite whose
# failures merge, and one that two jobs run costs a job and checks nothing
# more. See the comment on ci-frontend-packages.
GUARDS += check-test-scripts
GUARD_LANES.check-test-scripts := lint pre-push ci
check-test-scripts: ## Guard: every package `test` script is run by exactly one pull request CI job
	@python3 ./scripts/check-test-scripts.py --self-test
	@python3 ./scripts/check-test-scripts.py
