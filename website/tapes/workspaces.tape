# VHS Tape: workspace management demo
# Demonstrates real workspace operations against a running bowrain-server.
# Requires: start-server.sh to be run first (sets BOWRAIN_TOKEN).

Output output/workspaces.webm
Output output/workspaces.gif

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# Workspace management via the Bowrain API"
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

Type@50ms "bash scripts/ws-commands.sh ws_list"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Create a team workspace"
Enter
Sleep 500ms

Type@50ms "bash scripts/ws-commands.sh ws_create"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Create a project in the workspace"
Enter
Sleep 500ms

Type@50ms "bash scripts/ws-commands.sh ws_create_project"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# List projects in workspace"
Enter
Sleep 500ms

Type@50ms "bash scripts/ws-commands.sh ws_list_projects"
Enter
Sleep 2s

Sleep 1s
