GUARDS += check-go-trimpath
GUARD_LANES.check-go-trimpath := lint pre-push ci
check-go-trimpath: ## Guard: every Go command in make, scripts and CI builds with -trimpath
	@python3 ./scripts/check-go-trimpath.py --self-test
	@python3 ./scripts/check-go-trimpath.py
