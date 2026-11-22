# The agreement half runs offline. tool-versions.yml checks the pins
# against upstream on a schedule.
GUARDS += check-tool-versions
GUARD_LANES.check-tool-versions := pre-push ci
check-tool-versions: ## Guard: every copy of a tool pin names the same version
	@python3 ./scripts/check-tool-versions.py --self-test
	@python3 ./scripts/check-tool-versions.py --offline
