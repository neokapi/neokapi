# Every keychain access goes through core/keychain, which opens an in-memory
# keyring in a test binary, so no test reads or writes the developer's real
# keychain. It catches the other doors too: keyring libraries, the macOS
# `security` tool, the Secret Service, the Windows credential manager.
GUARDS += check-keychain-door
GUARD_LANES.check-keychain-door := lint pre-push ci
check-keychain-door: ## Guard: every keychain access goes through core/keychain, which a test binary keeps in memory
	@./scripts/check-keychain-door.sh --self-test
	@./scripts/check-keychain-door.sh
