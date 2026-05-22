# VHS tape — generated from bowrain/web/docs/walkthroughs/bowrain-getting-started.md
# Scene 2: push (terminal). Do not edit by hand.
# Requires: WALKTHROUGH_DIR set, BOWRAIN_BACKEND_URL + BOWRAIN_TOKEN exported,
# scene 1 (init) already run against the same WALKTHROUGH_DIR.

Output "02-push.webm"

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

Type "kapi push"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "kapi status"
Enter
Sleep 2s

Sleep 1s
