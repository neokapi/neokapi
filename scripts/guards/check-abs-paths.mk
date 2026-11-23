# Ungated: an absolute home path can land in any file.
GUARDS += check-abs-paths
GUARD_LANES.check-abs-paths := lint pre-push ci
check-abs-paths: ## Guard: no absolute home path (/Users/…, /home/…, C:\Users\…) in tracked files
	@./scripts/check-abs-paths.sh
