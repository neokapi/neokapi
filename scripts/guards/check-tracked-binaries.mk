GUARDS += check-tracked-binaries
GUARD_LANES.check-tracked-binaries := lint ci
check-tracked-binaries: ## Guard: no compiled executable (ELF/Mach-O/PE) is tracked in git
	@./scripts/check-tracked-binaries.sh
