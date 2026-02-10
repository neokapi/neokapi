# VHS Tape: kapi serve demo
# Demonstrates the local project server

Output output/serve.webm
Output output/serve.gif

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# Start a local translation editor in the browser"
Enter
Sleep 500ms

Type "kapi serve samples/website-translation.kaz"
Enter
Sleep 1500ms

Type "# → Importing project from website-translation.kaz..."
Enter
Sleep 500ms

Type "# → Starting local server on http://localhost:3000"
Enter
Sleep 500ms

Type "# → Opening browser..."
Enter
Sleep 2s

Type "# The web UI opens with the same editor as Bowrain"
Enter
Sleep 1500ms

Type "# No authentication required — localhost only"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Use a custom port"
Enter
Sleep 500ms

Type "kapi serve samples/software-ui.kaz --port 4000"
Enter
Sleep 1500ms

Type "# → Starting local server on http://localhost:4000"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Press Ctrl+C to stop — changes are saved back to the file"
Enter
Sleep 500ms

Type "^C"
Sleep 500ms

Type "# → Exporting changes to software-ui.kaz..."
Enter
Type "# → Done."
Enter
Sleep 2s

Sleep 1s
