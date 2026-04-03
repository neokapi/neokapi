# VHS Tape: walkthrough part 1 — project init
# Initializes a bowrain project in an existing codebase
# Requires: running bowrain-server, WALKTHROUGH_DIR set

Output output/walkthrough-init.webm

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "cd $WALKTHROUGH_DIR"
Enter
Sleep 500ms

Type "ls"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Initialize a bowrain project"
Enter
Sleep 500ms

Type "bowrain init"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "ls .bowrain/"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "bowrain status"
Enter
Sleep 3s

Sleep 1s
