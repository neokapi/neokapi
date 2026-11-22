GUARDS += check-cask-heredocs
GUARD_LANES.check-cask-heredocs := lint ci
check-cask-heredocs: ## Guard: the release casks carry no pre/postflight block and no retired vocabulary in desc; formulae use post_install_steps
	@./scripts/check-cask-heredocs.sh
