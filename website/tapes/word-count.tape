# VHS Tape: kapi word-count demo
# Demonstrates word counting for localization estimation

Output output/word-count.webm

Set FontSize 16
Set Width 900
Set Height 400
Set Theme "Dracula"
Set Padding 20

Type "# Count words in a JSON file"
Enter
Sleep 500ms

Type "kapi word-count samples/messages.json"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Word count helps estimate translation costs"
Enter
Sleep 1s
