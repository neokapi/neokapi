# Every keychain access goes through core/keychain, which opens an in-memory
# keyring in a test binary, so no test reads or writes the developer's real
# keychain. The sentinel half is check-test-isolation.
GUARDS += check-keychain-door
GUARD_LANES.check-keychain-door := lint pre-push ci
check-keychain-door: ## Guard: every keychain access goes through core/keychain, which a test binary keeps in memory
	@./scripts/check-test-isolation.sh --static
