# The terms, content memory, voice profiles and widened rules are projections
# of the workspace's operation log, written only by core/projector. A write that
# reaches one another way leaves a row a rebuild drops. The guard type-checks the
# Apache modules and proves itself on fixtures first.
#
# Ungated: a store write can be added in any Apache module, and only this
# guard, which type-checks them, notices one that bypasses the projector. CI
# runs it in the module-boundaries job and in each lint-tags leg.
GUARDS += check-projection-writes
GUARD_LANES.check-projection-writes := pre-push
check-projection-writes: i18n-catalogs ## Guard: only the projector writes a project's context stores
	@$(GO) run ./scripts/projectionguard -self-test
	@for c in $(GUARD_CONFIGS); do $(GO) run ./scripts/projectionguard -config $$c || exit 1; done
