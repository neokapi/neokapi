# VHS Tape: E2E walkthrough — pseudo-translate
# Demonstrates pseudo-translation on Docusaurus i18n JSON files.
# Requires: WALKTHROUGH_DIR set by generate.sh.

Output output/walkthrough-pseudo.webm
Output output/walkthrough-pseudo.gif

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# Step 2: Generate pseudo-translations for UI testing"
Enter
Sleep 500ms

Type "cd $WALKTHROUGH_DIR"
Enter
Sleep 500ms

Type "head -20 i18n/en/code.json"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "kapi pseudo-translate i18n/en/code.json --target-lang qps --output i18n/qps/code.json"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Check the pseudo-translated output"
Enter
Sleep 500ms

Type "head -20 i18n/qps/code.json"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# Add pseudo-translations to git"
Enter
Sleep 500ms

Type "git add i18n/qps"
Enter
Sleep 500ms

Type "git status"
Enter
Sleep 2s

Sleep 1s
