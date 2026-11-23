# Ungated: format drift can come from any Go file. The bowrain checks in
# pre-push are gated on ^bowrain/core/ and ^bowrain/plugin/ only, so the
# main bowrain module has no other local format gate. About a second for
# the whole tree.
GUARDS += check-gofmt
GUARD_LANES.check-gofmt := lint pre-push ci
check-gofmt: ## Guard: every tracked .go file is gofmt-clean (gofmt -l -s); `make fmt` fixes
	@./scripts/check-gofmt.sh
