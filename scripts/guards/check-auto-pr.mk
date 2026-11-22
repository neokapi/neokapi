# The delivery that carries a scheduled job's output to a reviewable pull
# request runs only in scheduled jobs, so nothing else on a pull request
# would notice it pushing to the wrong place or staging more than the job
# owns. Its self-test drives the real staging, push and refresh against
# scratch repositories with a real remote. About a second.
GUARDS += check-auto-pr
GUARD_LANES.check-auto-pr := pre-push ci
check-auto-pr: ## Guard: scheduled output is delivered as a reviewable pull request
	@./scripts/auto-pr.sh --self-test
