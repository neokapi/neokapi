# Verify a change to the Bowrain server or its stores

For Go under `bowrain/`, a `.sql` file, a migration, or the shared stores
(`terms/`, `memory/`, `core/storage/`). Run both halves: the suites pull
request CI skips, then the real server on PostgreSQL with Keycloak sign-in.

## 1. The suites CI skips on a pull request

Pull request CI passes `-short`, which skips every suite built on
`bowrain/testutil/pgtest`. Run them against a database of your own, on the
image CI uses. Use `postgres:16-alpine`: on the Debian `postgres:16` image
`TestBlockQuery_AfterIDWalksInIDOrder` (store) and
`TestReviewDeckPages_ComponentsAndDocuments` (server) fail, because glibc
sorts mixed-case ids differently from musl.

```bash
export PKG_CONFIG_PATH="$(brew --prefix icu4c)/lib/pkgconfig"   # macOS, for every go test
docker run -d --rm --name bowrain-test-pg -p 55432:5432 \
  -e POSTGRES_USER=bowrain -e POSTGRES_PASSWORD=bowrain -e POSTGRES_DB=bowrain_test \
  postgres:16-alpine
export BOWRAIN_TEST_POSTGRES_URL='postgres://bowrain:bowrain@localhost:55432/bowrain_test?sslmode=disable'
```

Set the variable even when Docker is available: the migration tests fall back
to `localhost:5432`, the local stack's database, and skip instead of failing.
Run from the repository root; name your changed packages in the first line.

```bash
(cd bowrain && go test -tags fts5 -count=1 ./store/ ./terms/ ./memory/)   # changed packages: 75 s
go test -tags fts5 -count=1 ./terms/... ./memory/... ./core/storage/...   # shared stores, SQLite: 6 s
(cd bowrain && go test -tags "fts5,integration" -count=1 -v ./migrations/) # migrations: 7 s
(cd bowrain && go list -e ./... | grep -v /apps/bowrain | xargs go test -tags fts5 -count=1)  # module: 5 to 10 min
make test-integration                                                      # integration lane: 3.5 min
make test-race-changed BASE_SHA=$(git merge-base origin/main HEAD)         # race, changed packages
```

- Do not use `make -C bowrain test-bowrain`. Outside CI it tests `./...`,
  including `bowrain/apps/bowrain`, whose `//go:embed all:frontend/dist` fails
  in a clean checkout (`pattern all:frontend/dist: no matching files found`).
  The `go list -e` line skips it, as CI does. Keep `xargs`: zsh passes `$(...)`
  as one word.
- `make test-integration` stops on the same embed. Create CI's placeholder
  first (git ignores it):
  `mkdir -p bowrain/apps/bowrain/frontend/dist && echo placeholder > bowrain/apps/bowrain/frontend/dist/index.html`
- An intended baseline schema change: add `-run TestBaselineSchemaMatchesGolden -update`
  to the migrations line and commit `bowrain/migrations/testdata/schema.golden`.
- `test-race-changed` diffs commits, so commit first. It runs `-short`.
- Grep a `-v` run for `SKIP` before you call a suite passed.

Remove the database afterwards: `docker rm -f bowrain-test-pg`.

## 2. Run the server and drive it

`bowrain/compose.full.yaml` runs the server, the worker and every service they
use in Docker. Give it a compose project of your own: the developer's stack is
`bowrain-local`, and `stack-down` deletes a project's volumes. If `docker ps`
shows `bowrain-local` containers up, they hold the ports; stop and ask.

```bash
export COMPOSE_PROJECT_NAME=bowrain-verify
make -C bowrain stack-up-e2e        # about 2 min with a warm Docker build cache
curl -s http://localhost:8080/api/v1/health
```

Healthy is `{"status":"ok",...}`. `/api/v1/ready` says `"unhealthy"` locally
because no AI provider is set; read its `components.database.status`, which
must be `"up"`. `stack-up-e2e` gives the `bowrain` realm password sign-in and
seeds a test user; it prints the user and password (the `E2E_USERNAME` and
`E2E_PASSWORD` defaults in `bowrain/Makefile`). Put them in `E2E_USER` and
`E2E_PASS`. Leave the stack up between verifications.

**A token through the real Keycloak.** This runs the browser sign-in with curl
and prints the `bowrain_session` cookie, which also works as a bearer token for
15 minutes.

```bash
bowrain_token() {  # usage: bowrain_token <username> <password>
  local jar form
  jar=$(mktemp)
  form=$(curl -sS -c "$jar" -b "$jar" -L http://localhost:8080/api/v1/auth/login \
    | grep -o 'action="[^"]*"' | head -1 \
    | sed -e 's/^action="//' -e 's/"$//' -e 's/&amp;/\&/g')
  curl -sS -c "$jar" -b "$jar" -L -o /dev/null \
    --data-urlencode "username=$1" --data-urlencode "password=$2" "$form"
  awk '$6=="bowrain_session"{print $7}' "$jar"
  rm -f "$jar"
}
API=http://localhost:8080/api/v1
TOKEN=$(bowrain_token "$E2E_USER" "$E2E_PASS")
curl -sS -H "Authorization: Bearer $TOKEN" $API/auth/me
# a new user has no workspace until onboarding; then create a project
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"slug":"e2e-reviewer"}' $API/auth/me/onboarding
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Verify","default_source_language":"en","target_languages":["fr"]}' \
  $API/e2e-reviewer/projects
```

**Rebuild after a Go change** (70 to 100 s; add `bowrain-worker` when the
change reaches job processing), then read the logs. `make stack-logs` follows
and never returns, so do not use it.

```bash
docker compose -f bowrain/compose.full.yaml up -d --build --wait bowrain-server
docker compose -f bowrain/compose.full.yaml logs --no-color --since 5m bowrain-server bowrain-worker
```

**Visibility.** When the diff touches who may see a route, call it as a member,
as a signed-in outsider and with no token. Seed the outsider (5 s):

```bash
E2E_USERNAME=outsider@example.com E2E_PASSWORD=outsider-password \
  docker compose -f bowrain/compose.full.yaml -f bowrain/compose.e2e.yaml run --rm keycloak-e2e
OTHER=$(bowrain_token outsider@example.com outsider-password)
curl -sS -o /dev/null -w 'member %{http_code}\n' -H "Authorization: Bearer $TOKEN" $API/e2e-reviewer/<project-id>
curl -sS -w ' outsider %{http_code}\n' -H "Authorization: Bearer $OTHER" $API/e2e-reviewer/<project-id>
curl -sS -w ' anonymous %{http_code}\n' $API/e2e-reviewer/<project-id>
```

Today this gives 200, 403 `not a member of this workspace`, 401 `missing authorization`.

**A migration over existing data.** Build the server at the branch's base,
start it on an empty database, load data, then start the changed build.

```bash
BASE=$(mktemp -d)
git archive "$(git merge-base origin/main HEAD)" | tar -x -C "$BASE"
docker build -q -f "$BASE/bowrain/docker/bowrain-server/Dockerfile" -t bowrain-verify/server:base "$BASE"   # 90 s
make -C bowrain stack-down
BOWRAIN_SERVER_IMAGE=bowrain-verify/server:base \
  docker compose -f bowrain/compose.full.yaml up -d --no-build --wait bowrain-server
docker compose -f bowrain/compose.full.yaml -f bowrain/compose.e2e.yaml run --rm keycloak-e2e
# sign in, onboard and create data with the calls above
docker compose -f bowrain/compose.full.yaml up -d --build --wait
```

Starting only `bowrain-server` keeps the worker, which also migrates, away
from the old database. The server opens every store before it logs
`starting Bowrain server`, so a failed migration exits before that line.
Confirm the line, a 200 on a route that reads the old rows, and the applied
versions, one `<subsystem>_schema_migrations` table per subsystem:
`docker compose -f bowrain/compose.full.yaml exec -T postgres psql -U bowrain -d bowrain -Atc "select max(version) from store_schema_migrations"`

**A push.** `push/commit` answers with a `push_id`, and the worker applies the
push afterwards. Poll until `status` is `completed` or `failed` before you
read results. `no_jobs` means a wrong id or project, never done.

```bash
curl -sS -H "Authorization: Bearer $TOKEN" "$API/<ws>/<project-id>/sync/main/status?push_id=<push_id>"
```

**Tear down.** `make -C bowrain stack-down` (5 s) removes the containers and
volumes of `$COMPOSE_PROJECT_NAME`. Confirm with `docker ps -a`.

## Evidence

The command, the status code and the response field the change affects, for
the changed case and one unchanged case. For a migration, the
`starting Bowrain server` line and the read of the old rows.
