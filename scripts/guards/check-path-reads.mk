# A project file reached by a path string follows any symbolic link on that
# path, so a link in a checkout can lead the call outside the project. The guard
# type-checks the packages that handle project content and reports each by-path
# file-system call outside the places scripts/pathguard lists with a reason,
# and proves itself on fixtures first.
#
# CI runs it in the module-boundaries job.
GUARDS += check-path-reads
GUARD_LANES.check-path-reads := lint pre-push
check-path-reads: i18n-catalogs go-embed-placeholders ## Guard: project content reaches a file through core/rootpath or the file home
	@$(GO) run ./scripts/pathguard -self-test
	@$(GO) run ./scripts/pathguard
