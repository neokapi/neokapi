#!/bin/bash
# Helper script for workspaces.tape VHS recording.
# VHS Type command cannot contain double quotes, so we wrap
# the API calls here and invoke them by function name.

BASE="http://localhost:8080/api/v1"
AUTH="Authorization: Bearer $BOWRAIN_TOKEN"

ws_list() {
  curl -s -H "$AUTH" "$BASE/workspaces" | jq
}

ws_create() {
  curl -s -X POST -H "$AUTH" -H 'Content-Type: application/json' \
    -d '{"name":"Acme Translations","slug":"acme"}' \
    "$BASE/workspaces" | jq .name
}

ws_create_project() {
  curl -s -X POST -H "$AUTH" -H 'Content-Type: application/json' \
    -d '{"name":"Website","source_locale":"en","target_locales":["fr","de"]}' \
    "$BASE/workspaces/acme/projects" | jq .name
}

ws_list_projects() {
  curl -s -H "$AUTH" "$BASE/workspaces/acme/projects" | jq '.[].name'
}

"$@"
