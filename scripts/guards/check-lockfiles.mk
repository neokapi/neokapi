# Every committed pnpm lockfile parses as pnpm reads it, gives each dependency
# an entry with an integrity hash, and holds no resolution but that hash from
# the registry the repository's .npmrc names.
GUARDS += check-lockfiles
GUARD_LANES.check-lockfiles := lint pre-push ci
GUARD_PATHS.check-lockfiles = (^|/)pnpm-lock\.yaml$$ (^|/)\.npmrc$$ ^scripts/check-lockfiles\.py$$ ^scripts/lib/pnpm_lockfile\.py$$
check-lockfiles: ## Guard: every pnpm lockfile resolves each package to an integrity hash from the configured registry
	@python3 ./scripts/check-lockfiles.py --self-test
	@python3 ./scripts/check-lockfiles.py
