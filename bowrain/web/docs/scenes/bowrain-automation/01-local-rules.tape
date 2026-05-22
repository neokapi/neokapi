# VHS tape — generated from bowrain/web/docs/walkthroughs/bowrain-automation.md
# Scene 1: local-rules (terminal). Do not edit by hand.
# Requires: WALKTHROUGH_DIR set with a .kapi recipe containing top-level hooks/automations.

Output "01-local-rules.webm"

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

Type "cat *.kapi"
Enter
Sleep 4s

Type ""
Enter
Sleep 500ms

Type "# Push triggers pre-push QA automatically"
Enter
Sleep 500ms

Type "kapi push"
Enter
Sleep 5s

Sleep 1s
