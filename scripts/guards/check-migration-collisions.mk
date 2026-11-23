# Ungated: two changes that each append "the next migration version" pass on
# their own and collide once both merge, and a database that recorded one
# skips the other for good. This compares every migration list with
# origin/main's (fetched when a shallow checkout lacks it) and fails on one
# version under two names. It cannot see a collision that reaches main after
# it ran; the migration runner, which records each migration's identity and
# refuses to start on a disagreement, is what protects a database. Seconds.
GUARDS += check-migration-collisions
GUARD_LANES.check-migration-collisions := lint pre-push ci
check-migration-collisions: ## Guard: no migration version collides with origin/main's
	@$(GO) run ./scripts/migrationguard -self-test
	@$(GO) run ./scripts/migrationguard
