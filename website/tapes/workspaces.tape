# VHS Tape: workspace management demo
# Demonstrates workspace operations via the REST API

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

Type "# First, authenticate"
Enter
Sleep 500ms

Type "kapi auth login --server http://localhost:8080"
Enter
Sleep 1s

Type "# → Logged in as admin@example.com"
Enter
Sleep 1s

Type ""
Enter
Sleep 500ms

Type "# List your workspaces"
Enter
Sleep 500ms

Type@50ms "curl -s -H 'Authorization: Bearer $TOKEN' http://localhost:8080/api/v1/workspaces | jq"
Enter
Sleep 1500ms

Type "# → [{name: Personal, slug: personal, role: owner}]"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Create a team workspace"
Enter
Sleep 500ms

Type@50ms "curl -s -X POST -H 'Authorization: Bearer $TOKEN' \"
Enter
Type@50ms "  -d '{name: Acme Translations, slug: acme}' \"
Enter
Type@50ms "  http://localhost:8080/api/v1/workspaces | jq .name"
Enter
Sleep 1500ms

Type "# → Acme Translations"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Create a project in the workspace"
Enter
Sleep 500ms

Type@50ms "curl -s -X POST -H 'Authorization: Bearer $TOKEN' \"
Enter
Type@50ms "  -d '{name: Website, source_locale: en, target_locales: [fr,de]}' \"
Enter
Type@50ms "  http://localhost:8080/api/v1/workspaces/acme/projects | jq .name"
Enter
Sleep 1500ms

Type "# → Website"
Enter
Sleep 2s

Sleep 1s
