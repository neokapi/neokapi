# The recorded walkthroughs are the only reader of the apps' testids, slots and
# rail labels from outside the apps, and nothing compiles them together. A
# rename is silent until a re-record: one sweep found nine of twelve walks
# broken, four of them on a single word.
#
# Ungated: a walk selector dies in the app, not in the recorder that
# names it, so gating this on the recorder's own path would never fire.
GUARDS += check-walk-selectors
GUARD_LANES.check-walk-selectors := lint pre-push ci
check-walk-selectors: ## Guard: every selector a recorded walk drives is still rendered by an app
	@./scripts/check-walk-selectors.sh
