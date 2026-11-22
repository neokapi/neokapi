# The fuzz runner runs only in the nightly fuzz.yml. Its self-test fuzzes
# a scratch module holding a target that fails, and checks that the run
# goes on to the other targets, names the saved input and exits 1. About
# 15 seconds.
GUARDS += check-fuzz-runner
GUARD_LANES.check-fuzz-runner := pre-push ci
check-fuzz-runner: ## Guard: the nightly fuzz runner reports a failing target
	@./scripts/go-fuzz.sh --self-test
