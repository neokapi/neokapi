# VHS Tape: kapi serve demo
# Demonstrates the local project server command

Output output/serve.webm
Output output/serve.gif

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# Local project server — like 'jupyter notebook' for translations"
Enter
Sleep 500ms

Type "kapi serve --help"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# Open a .kaz project in the browser"
Enter
Sleep 500ms
Type "# kapi serve website.kaz"
Enter
Sleep 500ms
Type "# → Starting local project server at http://localhost:3000"
Enter
Type "# → Project: /Users/demo/website.kaz"
Enter
Type "# → Press Ctrl+C to stop."
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Use a custom port"
Enter
Sleep 500ms
Type "# kapi serve website.kaz --port 4000"
Enter
Sleep 500ms
Type "# → Starting local project server at http://localhost:4000"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Serve the current directory"
Enter
Sleep 500ms
Type "# kapi serve"
Enter
Sleep 500ms
Type "# → Starting local project server at http://localhost:3000"
Enter
Type "# → Project: /Users/demo/my-project"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# On Ctrl+C, changes are exported back to the .kaz file"
Enter
Sleep 500ms
Type "# → Saving changes back to website.kaz"
Enter
Type "# → Saved project to website.kaz"
Enter
Sleep 2s

Sleep 1s
