# The lab's transcripts are COMMITTED, so they never pass through the publish
# script that checks the skill eval's. Same shape of risk and a shorter path to a
# public repository: an agent with bypassPermissions, a shell and $HOME recorded
# whole. Checked here, where every commit sees it.
GUARDS += check-eval-publishable
GUARD_LANES.check-eval-publishable := lint ci
check-eval-publishable: ## Guard: no credential shape in a committed eval transcript
	@./scripts/check-eval-publishable.sh web/static/authoring-lab
