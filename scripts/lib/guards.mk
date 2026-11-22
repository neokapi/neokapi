# ── Repo guards ──────────────────────────────────────────────────────────────
#
# The guard registry. Each file scripts/guards/<target>.mk holds one guard:
#
#   # Why the guard exists, and why it is gated or not.
#   GUARDS += check-foo
#   GUARD_LANES.check-foo := lint pre-push ci
#   GUARD_PATHS.check-foo = ^some/dir/ \.ext$
#   check-foo: ## Guard: what it asserts
#   	@./scripts/check-foo.sh --self-test
#   	@./scripts/check-foo.sh
#
# The lanes are where the guard runs: `lint` is a prerequisite of `make lint`,
# `pre-push` runs in scripts/pre-push-check.sh, and `ci` runs in the Repo guards
# job of .github/workflows/ci.yml. GUARD_PATHS is optional and applies to
# pre-push only: a list of extended regular expressions, matched against the
# changed paths. A guard with none runs on every pre-push. Define GUARD_PATHS
# with `=`, so a `$` stays literal; scripts/run-guards.sh reads it unexpanded.
#
# Every consumer enumerates this directory, so adding a guard adds one file
# and edits no shared line. scripts/check-guard-registry.py checks that each
# file is well formed and that no guard is wired anywhere else.

GUARD_LANE_NAMES := lint pre-push ci
GUARD_FILES := $(sort $(wildcard scripts/guards/*.mk))
include $(GUARD_FILES)

# The guards whose GUARD_LANES names lane $(1), in file order.
guards-in = $(foreach t,$(GUARDS),$(if $(filter $(1),$(GUARD_LANES.$(t))),$(t)))

GUARDS_LINT := $(call guards-in,lint)

.PHONY: $(GUARDS) guard-list

# One line per guard of LANE: the target, a tab, then its GUARD_PATHS.
guard-list: ## Print the guards of one lane (LANE=lint|pre-push|ci) with their pre-push paths
	@$(if $(filter $(LANE),$(GUARD_LANE_NAMES)),,echo "guard-list: LANE must be one of: $(GUARD_LANE_NAMES)" >&2; exit 2)
	@$(foreach t,$(call guards-in,$(LANE)),printf '%s\t%s\n' '$(t)' '$(value GUARD_PATHS.$(t))';)
