# VHS Tape: workspace management demo
# Demonstrates real workspace operations against a running gokapi-server.
# Requires: start-server.sh to be run first (sets GOKAPI_TOKEN).

Output output/workspaces.webm
Output output/workspaces.gif

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# Workspace management via the gokapi API"
Enter
Sleep 500ms

Type "# Check server health"
Enter
Sleep 500ms

Type@50ms "curl -s http://localhost:8080/api/v1/health | jq"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# List your workspaces"
Enter
Sleep 500ms

Type@50ms "curl -s -H \"Authorization: Bearer $GOKAPI_TOKEN\" http://localhost:8080/api/v1/workspaces | jq"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Create a team workspace"
Enter
Sleep 500ms

Type@50ms "curl -s -X POST -H \"Authorization: Bearer $GOKAPI_TOKEN\" \"
Enter
Type@50ms "  -H 'Content-Type: application/json' \"
Enter
Type@50ms "  -d '{\"name\":\"Acme Translations\",\"slug\":\"acme\"}' \"
Enter
Type@50ms "  http://localhost:8080/api/v1/workspaces | jq .name"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Create a project in the workspace"
Enter
Sleep 500ms

Type@50ms "curl -s -X POST -H \"Authorization: Bearer $GOKAPI_TOKEN\" \"
Enter
Type@50ms "  -H 'Content-Type: application/json' \"
Enter
Type@50ms "  -d '{\"name\":\"Website\",\"source_locale\":\"en\",\"target_locales\":[\"fr\",\"de\"]}' \"
Enter
Type@50ms "  http://localhost:8080/api/v1/workspaces/acme/projects | jq .name"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# List projects in workspace"
Enter
Sleep 500ms

Type@50ms "curl -s -H \"Authorization: Bearer $GOKAPI_TOKEN\" http://localhost:8080/api/v1/workspaces/acme/projects | jq '.[].name'"
Enter
Sleep 2s

Sleep 1s
