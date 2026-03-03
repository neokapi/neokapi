# VHS Tape: walkthrough part 2 — pseudo-translate
# Runs pseudo-translation flow on the project
# Requires: WALKTHROUGH_DIR set

Output output/walkthrough-pseudo.webm

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "cd $WALKTHROUGH_DIR"
Enter
Sleep 500ms

Type "# Run pseudo-translation flow"
Enter
Sleep 500ms

Type "bowrain flow run pseudo"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "bowrain status"
Enter
Sleep 3s

Sleep 1s
