# A Go test that set no KAPI_CONFIG_DIR once rewrote the developer's
# providers.json. Every per-user location now resolves to a directory of the
# test binary's own (core/dataroot.TestUserDir). This runs the packages that
# resolve those locations with HOME at an empty directory and fails if a test
# wrote there. It takes minutes, so pre-push runs it when those paths change.
GUARDS += check-test-isolation
GUARD_LANES.check-test-isolation := pre-push
GUARD_PATHS.check-test-isolation = ^core/dataroot/ ^core/keychain/ ^core/credentials/ ^core/channel/ ^core/flow/ ^host/ ^cli/selfupdate/ ^apps/kapi-desktop/backend/ ^bowrain/credentials/ ^bowrain/apps/bowrain/backend/ ^scripts/check-test-isolation\.sh$
check-test-isolation: ## Guard: Go tests write nothing under the user's home (config, data, caches, plugins)
	@./scripts/check-test-isolation.sh
