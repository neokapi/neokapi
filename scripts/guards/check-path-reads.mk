# A project file reached by a path string follows any symbolic link on that
# path, so a link in a checkout can lead the call outside the project. The guard
# type-checks every package of every module in go.work, except the few
# scripts/pathguard excludes with a reason, and reports each by-path
# file-system call that carries no `//pathguard: <reason>` comment. It proves
# itself on fixtures and on a violation planted in every covered package first.
#
# CI runs it in the module-boundaries job and in each lint-tags leg.
GUARDS += check-path-reads
GUARD_LANES.check-path-reads := lint pre-push
check-path-reads: i18n-catalogs go-embed-placeholders ## Guard: project content reaches a file through core/rootpath or the file home
	@$(GO) run ./scripts/pathguard -self-test
	@for c in $(GUARD_CONFIGS); do $(GO) run ./scripts/pathguard -config $$c || exit 1; done
