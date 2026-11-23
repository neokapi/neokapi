# Ungated: a camelCase json tag on an output type changes a field name
# every script and agent reads. About a second.
GUARDS += check-json-tags
GUARD_LANES.check-json-tags := lint pre-push ci
check-json-tags: ## Guard: machine-readable output uses snake_case json field names
	@./scripts/check-json-tags.sh
