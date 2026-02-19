# VHS Tape: E2E walkthrough — kapi init (anonymous project)
# Demonstrates creating an anonymous project on the server.
# Requires: WALKTHROUGH_DIR set by generate.sh, KAPI_SERVER_URL set.

Output output/walkthrough-init.webm
Output output/walkthrough-init.gif

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# Step 1: Initialize a project"
Enter
Sleep 500ms

Type "cd $WALKTHROUGH_DIR"
Enter
Sleep 500ms

Type "kapi init --anonymous --name 'Docusaurus E2E Demo' --source en"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# Check the generated config"
Enter
Sleep 500ms

Type "cat .kapi/config.yaml"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# Add .kapi/ to git"
Enter
Sleep 500ms

Type "git add .kapi"
Enter
Sleep 500ms

Type "git status"
Enter
Sleep 2s

Sleep 1s
