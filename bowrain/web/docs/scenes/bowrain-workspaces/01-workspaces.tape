# VHS tape — generated from bowrain/web/docs/walkthroughs/bowrain-workspaces.md
# Scene 1: workspaces (terminal). Do not edit by hand.
# Requires: BOWRAIN_BACKEND_URL set, BOWRAIN_TOKEN exported.

Output "01-workspaces.webm"

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# List workspaces via the REST API"
Enter
Sleep 500ms

Type "kapi workspace list"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Create a workspace"
Enter
Sleep 500ms

Type "kapi workspace create --name 'Demo Workspace'"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "kapi workspace list"
Enter
Sleep 2s

Sleep 1s
