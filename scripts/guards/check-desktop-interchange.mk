# Ungated: the interchange chrome came back once as one adapter method
# and one header action, and neither touched a file this check could have
# been gated on.
GUARDS += check-desktop-interchange
GUARD_LANES.check-desktop-interchange := lint pre-push ci
check-desktop-interchange: ## Guard: the Kapi Desktop surface names no interchange format (TMX/XLIFF/termbase)
	@./scripts/check-desktop-interchange.sh
