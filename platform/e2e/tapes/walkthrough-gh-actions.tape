# VHS Tape: walkthrough part 7 — CI/CD with GitHub Actions
# Shows the GitHub Actions workflow for automated sync
# Requires: WALKTHROUGH_DIR set

Output output/walkthrough-gh-actions.webm

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "cd $WALKTHROUGH_DIR"
Enter
Sleep 500ms

Type "# GitHub Actions workflow for automated translation sync"
Enter
Sleep 500ms

Type "cat .github/workflows/bowrain-sync.yml"
Enter
Sleep 5s

Type ""
Enter
Sleep 500ms

Type "# This runs on every push to main that changes source content"
Enter
Sleep 1s

Type "# bowrain sync handles: push → translate → pull"
Enter
Sleep 1s

Type ""
Enter
Sleep 500ms

Type "# View the project config"
Enter
Sleep 500ms

Type "cat .bowrain/config.yaml"
Enter
Sleep 5s

Sleep 1s
