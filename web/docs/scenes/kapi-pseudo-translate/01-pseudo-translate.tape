# VHS tape — generated from web/docs/walkthroughs/kapi-pseudo-translate.md
# Scene 1: pseudo-translate (terminal). Do not edit by hand.

Output "01-pseudo-translate.webm"

Set FontSize 16
Set Width 900
Set Height 550
Set Theme "Dracula"
Set Padding 20

Type "# Pseudo-translate to find UI truncation bugs early"
Enter
Sleep 500ms

Type "cat fixtures/messages.json"
Enter
Sleep 1s

Type "kapi pseudo-translate fixtures/messages.json --target-lang fr"
Enter
Sleep 1500ms

Type "cat out/messages.json"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Diacritics make missing or truncated strings visible at a glance"
Enter
Sleep 1s
