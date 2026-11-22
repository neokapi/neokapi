# The desktop update feeds are published by a script that runs only on a
# release tag, and a release candidate exercises only its beta half. Its
# self-test runs it for a final and a candidate on both tracks and checks
# which feeds reach the registry.
GUARDS += check-publish-appcast
GUARD_LANES.check-publish-appcast := ci
check-publish-appcast: ## Guard: a final tag publishes the stable update feed on both tracks
	@./scripts/publish-appcast.sh --self-test
