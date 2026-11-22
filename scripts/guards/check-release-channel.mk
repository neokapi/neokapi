# Both release tracks resolve their channel through one script, over tag
# shapes that differ between them. Its self-test runs every shape and
# checks that the workflows still call it: an inline copy of the test is
# how the bowrain track came to read a prefix hyphen as a prerelease.
GUARDS += check-release-channel
GUARD_LANES.check-release-channel := ci
check-release-channel: ## Guard: a final tag publishes stable on both release tracks
	@./scripts/release-channel.sh --self-test
