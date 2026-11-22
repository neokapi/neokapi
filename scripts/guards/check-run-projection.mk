# Ungated: a hand-rolled run walk is written wherever someone needs
# "just the text", and it fails by showing less content rather than by
# failing. About a second.
GUARDS += check-run-projection
GUARD_LANES.check-run-projection := lint pre-push ci
check-run-projection: ## Guard: a Run sequence is projected through a declared RunSpec, never a hand-rolled walk
	@./scripts/check-run-projection.sh
