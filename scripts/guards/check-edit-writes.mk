# A format writer pointed at a file writes a whole document to disk. The guard
# type-checks every module that writes documents and reports each such call
# outside the format packages and the places scripts/editguard lists with what
# each writes, and proves itself on fixtures first.
#
# CI runs it in the module-boundaries job and in each lint-tags leg.
GUARDS += check-edit-writes
GUARD_LANES.check-edit-writes := lint pre-push
check-edit-writes: i18n-catalogs go-embed-placeholders ## Guard: a document reaches a file only where scripts/editguard lists it
	@$(GO) run ./scripts/editguard -self-test
	@for c in $(GUARD_CONFIGS); do $(GO) run ./scripts/editguard -config $$c || exit 1; done
