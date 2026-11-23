# The plugin formula generator runs only on a `<plugin>-v*` tag, so nothing
# before a release exercises it. Its self-test renders every plugin from a
# synthetic checksum set and checks that an incomplete one is refused,
# since a formula with an empty platform block installs nothing and
# reports success.
GUARDS += check-brew-plugin-formula
GUARD_LANES.check-brew-plugin-formula := ci
check-brew-plugin-formula: ## Guard: a plugin's Homebrew formula renders from its release
	@./scripts/gen-brew-plugin-formula.sh --self-test
