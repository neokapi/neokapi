# VHS Tape: walkthrough part 3 — push
# Pushes local changes to Bowrain Server
# Requires: running bowrain-server, WALKTHROUGH_DIR set

Output output/walkthrough-push.webm

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "cd $WALKTHROUGH_DIR"
Enter
Sleep 500ms

Type "# Push changes to server"
Enter
Sleep 500ms

Type "bowrain push"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "bowrain status"
Enter
Sleep 2s

Sleep 1s
