# VHS tape — generated from bowrain/web/docs/walkthroughs/bowrain-getting-started.md
# Scene 4: sync (terminal). Do not edit by hand.
# Requires: WALKTHROUGH_DIR set, scenes 1+2+3 already run.

Output "04-sync.webm"

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

Type "kapi sync --timeout 2m"
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

Sleep 1s
