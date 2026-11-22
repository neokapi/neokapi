GUARDS += check-image-user
GUARD_LANES.check-image-user := lint pre-push ci
check-image-user: ## Guard: the server and worker images run as an unprivileged numeric user
	@./scripts/check-image-user.sh --self-test
	@./scripts/check-image-user.sh
