# VHS Tape: workspaces demo
# Shows workspace management via API
# Requires: running bowrain-server

Output output/workspaces.webm

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# List workspaces"
Enter
Sleep 500ms

Type "bash scripts/ws-commands.sh ws_list"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Create a workspace"
Enter
Sleep 500ms

Type "bash scripts/ws-commands.sh ws_create"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Create a project in the workspace"
Enter
Sleep 500ms

Type "bash scripts/ws-commands.sh ws_create_project"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# List projects"
Enter
Sleep 500ms

Type "bash scripts/ws-commands.sh ws_list_projects"
Enter
Sleep 2s

Sleep 1s
