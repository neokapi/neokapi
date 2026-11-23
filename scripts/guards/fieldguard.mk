# A block stores its content as peer editions in Block.Editions and lists the
# editions its document holds in Block.Native (docs/internals/edit-model.md,
# section 6.2). The guard type-checks every module with its tests and fails on
# a use of either field outside core/model, a test and the plugin wire, so
# every other package reaches the editions through the accessors and the
# storage can change inside core/model alone. A test may write the storage to
# plant a state no accessor produces; those uses are counted. The desktop
# placeholder bundle lets its main package type-check.
#
# CI runs it in a job of its own, the edition storage guard, because it takes
# minutes and needs the GTK and WebKit headers.
GUARDS += fieldguard
GUARD_LANES.fieldguard := lint pre-push
fieldguard: kapi-desktop-lint-deps ## Gate: no code outside core/model and its tests reaches Block.Editions or Block.Native past the edition accessors
	@$(GO) run ./scripts/fieldguard -self-test
	@# plugins/pdfium type-checks only with PDFium on pkg-config, and a package
	@# that does not type-check fails the guard.
	@pc="$$(./scripts/pdfium-pkgconfig.sh "$(PDFIUM_CACHE_DIR)")" && \
		PKG_CONFIG_PATH="$$pc$${PKG_CONFIG_PATH:+:$$PKG_CONFIG_PATH}" $(GO) run ./scripts/fieldguard $(FIELDGUARD_ARGS)
