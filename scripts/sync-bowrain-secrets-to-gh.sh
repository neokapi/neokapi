#!/usr/bin/env bash
#
# sync-bowrain-secrets-to-gh.sh — provision a fresh BOWRAIN_TOKEN and
# write it (plus BOWRAIN_BACKEND_URL + BOWRAIN_WORKSPACE) to GitHub
# Action secrets/variables for the current repository.
#
# Composes the building blocks:
#   - fetch-bowrain-admin-token.sh   (mint admin token from Azure)
#   - provision-bowrain-token.sh     (provision workspace API token)
#
# Nothing is written to disk locally. To use the token in a local shell:
#
#   export BOWRAIN_TOKEN=$(scripts/provision-bowrain-token.sh \
#       --workspace agent-sandbox-$USER)
#
# Or wire it via direnv in a .envrc that calls the same script.
#
# Usage
# -----
#   scripts/sync-bowrain-secrets-to-gh.sh \
#     --rg bowrain-dev-rg --env dev \
#     --workspace agent-sandbox
#
# Flags
# -----
#   --rg <rg>            Azure resource group (passed to fetch-admin-token; required)
#   --env dev|prod       Infra environment (default: dev)
#   --workspace <slug>   Workspace slug (default: agent-sandbox-<sanitized-USER>)
#   --server <url>       Backend URL (default: https://dev.bowrain.cloud)
#   --token-name <name>  Token name (default: agent-bot)
#   --expires-days <n>   Token expiry (default: 90)
#   --reset              Delete + recreate the workspace before provisioning

set -euo pipefail

AZ_RG="${AZ_RESOURCE_GROUP:-}"
AZ_ENV="${AZ_INFRA_ENV:-dev}"
SERVER_URL="${BOWRAIN_BACKEND_URL:-https://dev.bowrain.cloud}"
WORKSPACE_SLUG=""
TOKEN_NAME="agent-bot"
TOKEN_EXPIRES_DAYS=90
DO_RESET=false

while [ $# -gt 0 ]; do
  case "$1" in
    --rg)            AZ_RG="$2"; shift ;;
    --env)           AZ_ENV="$2"; shift ;;
    --workspace)     WORKSPACE_SLUG="$2"; shift ;;
    --server)        SERVER_URL="$2"; shift ;;
    --token-name)    TOKEN_NAME="$2"; shift ;;
    --expires-days)  TOKEN_EXPIRES_DAYS="$2"; shift ;;
    --reset)         DO_RESET=true ;;
    -h|--help)       sed -n '1,/^set -euo/p' "$0" | sed 's/^# \?//' | head -n -1; exit 0 ;;
    *)               echo "unknown flag: $1" >&2; exit 2 ;;
  esac
  shift
done

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing tool: $1" >&2; exit 1; }; }
need gh
need git

if [ -z "$WORKSPACE_SLUG" ]; then
  user_part="$(echo "${USER:-anon}" | tr -dc 'a-z0-9' | head -c 16)"
  WORKSPACE_SLUG="agent-sandbox-${user_part}"
fi

[ -n "$AZ_RG" ] || { echo "missing --rg <resource-group>" >&2; exit 2; }

REPO_ROOT="$(git rev-parse --show-toplevel)"
FETCH_ADMIN="$REPO_ROOT/scripts/fetch-bowrain-admin-token.sh"
PROVISION="$REPO_ROOT/scripts/provision-bowrain-token.sh"
[ -x "$FETCH_ADMIN" ] || chmod +x "$FETCH_ADMIN"
[ -x "$PROVISION" ]  || chmod +x "$PROVISION"

echo "==> Minting admin token (env=$AZ_ENV, rg=$AZ_RG)"
ADMIN_TOKEN="$("$FETCH_ADMIN" --rg "$AZ_RG" --env "$AZ_ENV")"
[ -n "$ADMIN_TOKEN" ] || { echo "admin token mint failed" >&2; exit 1; }

echo "==> Provisioning workspace API token (workspace=$WORKSPACE_SLUG)"
provision_args=(--workspace "$WORKSPACE_SLUG"
                --server "$SERVER_URL"
                --token-name "$TOKEN_NAME"
                --expires-days "$TOKEN_EXPIRES_DAYS")
$DO_RESET && provision_args+=(--reset)
BOWRAIN_TOKEN="$(BOWRAIN_ADMIN_TOKEN="$ADMIN_TOKEN" "$PROVISION" "${provision_args[@]}")"
[ -n "$BOWRAIN_TOKEN" ] || { echo "token provisioning failed" >&2; exit 1; }

echo "==> Writing GitHub Action secret + variables"
printf '%s' "$BOWRAIN_TOKEN" | gh secret set BOWRAIN_TOKEN --body -
gh variable set BOWRAIN_BACKEND_URL --body "$SERVER_URL"     2>/dev/null \
  || echo "  ⚠ could not set BOWRAIN_BACKEND_URL variable" >&2
gh variable set BOWRAIN_WORKSPACE   --body "$WORKSPACE_SLUG" 2>/dev/null \
  || echo "  ⚠ could not set BOWRAIN_WORKSPACE variable" >&2

echo
echo "Done."
echo "  GH secret: BOWRAIN_TOKEN ($TOKEN_EXPIRES_DAYS-day expiry)"
echo "  GH vars:   BOWRAIN_BACKEND_URL=$SERVER_URL, BOWRAIN_WORKSPACE=$WORKSPACE_SLUG"
echo
echo "For local use, run instead:"
echo "  export BOWRAIN_BACKEND_URL=$SERVER_URL"
echo "  export BOWRAIN_WORKSPACE=$WORKSPACE_SLUG"
echo "  export BOWRAIN_TOKEN=\$(BOWRAIN_ADMIN_TOKEN=\$($FETCH_ADMIN --rg $AZ_RG --env $AZ_ENV) \\"
echo "                          $PROVISION --workspace $WORKSPACE_SLUG)"
