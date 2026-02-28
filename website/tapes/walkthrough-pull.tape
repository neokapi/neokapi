# VHS Tape: E2E walkthrough — brain pull
# Demonstrates pulling translated content from Bowrain Server.
# Requires: WALKTHROUGH_DIR set by generate.sh, KAPI_SERVER_URL set.

Output output/walkthrough-pull.webm

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "cd $WALKTHROUGH_DIR"
Enter
Sleep 500ms

Type "# Step 7: Pull translations back to codebase"
Enter
Sleep 500ms

Type "brain pull --locale nb --locale fr"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# Check the translated files"
Enter
Sleep 500ms

Type "head -20 i18n/nb/code.json"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "head -20 i18n/fr/code.json"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Build the Norwegian version of the Docusaurus site"
Enter
Sleep 500ms

Type "npm run build -- --locale nb"
Enter
Sleep 5s

Sleep 1s
