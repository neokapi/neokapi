# VHS Tape: E2E walkthrough — kapi push
# Demonstrates pushing source content to Bowrain Server.
# Requires: WALKTHROUGH_DIR set by generate.sh, KAPI_SERVER_URL set.

Output output/walkthrough-push.webm
Output output/walkthrough-push.gif

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "cd $WALKTHROUGH_DIR"
Enter
Sleep 500ms

Type "# Step 3: Push content to Bowrain Server"
Enter
Sleep 500ms

Type "kapi status"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "kapi push"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# Verify the push"
Enter
Sleep 500ms

Type "kapi status"
Enter
Sleep 2s

Sleep 1s
