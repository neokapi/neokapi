GUARDS += check-reliability-checklist
GUARD_LANES.check-reliability-checklist := lint pre-push ci
check-reliability-checklist: ## Guard: the reliability checklist parses, validates, and each trigger selects its fixture
	@python3 ./scripts/reliability-checklist.py --self-test
