# The packager runs only on a release tag too. Its self-test packages a
# dummy tree and reads the archives back, because an archive that
# misdeclares itself looks green everywhere else: the release publishes,
# the formulae install, and only `kapi plugins list` on a user's machine
# reports the version.
GUARDS += check-package-cli
GUARD_LANES.check-package-cli := ci
check-package-cli: ## Guard: a released archive declares the version it was cut at
	@./scripts/package-cli.sh --self-test
