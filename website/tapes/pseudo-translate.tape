# VHS Tape: kapi pseudo-translate demo
# Demonstrates pseudo-translation for UI testing

Output output/pseudo-translate.webm

Set FontSize 16
Set Width 900
Set Height 550
Set Theme "Dracula"
Set Padding 20

Type "# Generate pseudo-translations for testing"
Enter
Sleep 500ms

Type "cat samples/messages.json"
Enter
Sleep 1s

Type "kapi pseudo-translate samples/messages.json --target-lang fr"
Enter
Sleep 1500ms

Type "cat out/messages.json"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Pseudo-translation helps find UI truncation issues"
Enter
Sleep 1s
