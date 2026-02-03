# VHS Tape: kapi convert demo
# Demonstrates format conversion between JSON and YAML

Output docs/tapes/output/convert.webm
Output docs/tapes/output/convert.gif

Set FontSize 16
Set Width 900
Set Height 500
Set Theme "Dracula"
Set Padding 20

Type "# Convert JSON to YAML"
Enter
Sleep 500ms

Type "cat samples/messages.json"
Enter
Sleep 1s

Type "kapi convert samples/messages.json -f json --to yaml"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Convert back to JSON"
Enter
Sleep 500ms

Type "kapi convert samples/messages.json -f json --to yaml | kapi convert -f yaml --to json"
Enter
Sleep 2s

Sleep 1s
