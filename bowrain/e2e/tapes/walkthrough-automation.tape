# VHS Tape: walkthrough part 6 — local automation rules
# Shows automation rules in .bowrain/config.yaml
# Requires: WALKTHROUGH_DIR set

Output output/walkthrough-automation.webm

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "cd $WALKTHROUGH_DIR"
Enter
Sleep 500ms

Type "# View local automation rules"
Enter
Sleep 500ms

Type "cat .bowrain/config.yaml"
Enter
Sleep 4s

Type ""
Enter
Sleep 500ms

Type "# Push triggers pre-push QA automatically"
Enter
Sleep 500ms

Type "bowrain push"
Enter
Sleep 5s

Type ""
Enter
Sleep 500ms

Type "bowrain status"
Enter
Sleep 3s

Sleep 1s
