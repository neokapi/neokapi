# Go flags every make-driven Go command in this repository shares.
#
# The root Makefile and bowrain/Makefile include this file, and both export
# GOFLAGS, so every `go` a recipe or a script under make runs inherits it.
#
# -trimpath: without it, cmd/go hashes each package's absolute directory into
# the build cache key, so every git worktree compiles the whole repository
# into cache entries of its own and none are shared. With it, two checkouts of
# one commit share every compiled package and every test result.
#
# The flag is appended once, after whatever GOFLAGS the caller set. CI sets
# the same flag (.github/actions/setup-go-modules and the workflows' env), and
# so do agent sessions (.claude/settings.json). scripts/check-go-trimpath.py
# fails when one of them drifts.
ifeq ($(filter -trimpath,$(GOFLAGS)),)
GOFLAGS := $(strip $(GOFLAGS) -trimpath)
endif
export GOFLAGS
