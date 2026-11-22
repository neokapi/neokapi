# The docs jobs build two sites in two locales and sit late in CI, so an
# unresolvable doc id costs a full round trip to learn. This says it in a
# second, on every pull request, for both sidebars at once.
GUARDS += check-sidebar-ids
GUARD_LANES.check-sidebar-ids := lint ci
check-sidebar-ids: ## Guard: every doc id a Docusaurus sidebar names resolves to a page
	@./scripts/check-sidebar-ids.sh
