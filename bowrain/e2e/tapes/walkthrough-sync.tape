# VHS Tape: walkthrough part 5 — sync
# Runs bowrain sync (push + wait for translations + pull)
# Requires: running bowrain-server, WALKTHROUGH_DIR set

Output output/walkthrough-sync.webm

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "cd $WALKTHROUGH_DIR"
Enter
Sleep 500ms

Type "# One command to push, translate, and pull"
Enter
Sleep 500ms

Type "bowrain sync --timeout 2m"
Enter
Sleep 8s

Type ""
Enter
Sleep 500ms

Type "# Check the results"
Enter
Sleep 500ms

Type "ls i18n/"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "head -20 i18n/nb/code.json"
Enter
Sleep 3s

Sleep 1s
