# The CI test jobs run through scripts/check-temp-leaks.sh, which fails a run
# that leaves files in its temporary directory. This target proves the script.
GUARDS += check-temp-leaks
GUARD_LANES.check-temp-leaks := lint pre-push ci
check-temp-leaks: ## Guard: the temporary-file leak check passes its self-test
	@./scripts/check-temp-leaks.sh --self-test
