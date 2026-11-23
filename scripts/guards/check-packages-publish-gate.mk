GUARDS += check-packages-publish-gate
GUARD_LANES.check-packages-publish-gate := lint ci
check-packages-publish-gate: ## Guard: the apt/yum repository publishes only when PUBLISH_PACKAGES is "true"
	@./scripts/check-packages-publish-gate.sh
