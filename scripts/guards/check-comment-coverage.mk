# The recipe checks the comments of every tracked file in a family it wires, so
# a file no collection claims and no defaults.exclude pattern covers has comments
# nothing reads. The guard proves itself on fixtures, then checks this repository.
#
# Ungated: a directory added anywhere can hold files whose comments no
# kapi.yaml collection reaches, and only this guard notices.
GUARDS += check-comment-coverage
GUARD_LANES.check-comment-coverage := lint pre-push ci
check-comment-coverage: ## Guard: every tracked Go, code, YAML and markup file is claimed by a kapi.yaml collection or excluded
	@CGO_ENABLED=0 $(GO) run ./scripts/commentcoverage -self-test
	@CGO_ENABLED=0 $(GO) run ./scripts/commentcoverage
