# A map key or a hash input joined from parts with a delimiter (a + "\x00" + b)
# gives two different tuples one key or identity when a part holds the
# delimiter. The guard type-checks the Apache modules and Bowrain and reports
# each such join: a key takes a struct or array instead, and a hash input or
# id takes core/partjoin. A persisted identity is exempt where it is built, with
# //keyguard:persisted naming where it is stored and the test that pins it. It
# proves itself on fixtures first.
#
# It type-checks Go, so CI runs it beside the edit guard: the host
# configuration in the module-boundaries job and each other one in its
# lint-tags leg.
GUARDS += check-delimiter-keys
GUARD_LANES.check-delimiter-keys := lint pre-push
check-delimiter-keys: i18n-catalogs go-embed-placeholders ## Guard: no map key or hash input is joined from parts with a delimiter
	@$(GO) run ./scripts/keyguard -self-test
	@for c in $(GUARD_CONFIGS); do $(GO) run ./scripts/keyguard -config $$c || exit 1; done
