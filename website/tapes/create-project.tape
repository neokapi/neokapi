# VHS Tape: End-to-end project workflow
# Shows word count and pseudo-translation of source files

Output output/create-project.webm
Output output/create-project.gif

Set FontSize 15
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# End-to-end project workflow with kapi"
Enter
Sleep 500ms

Type "# First, let's look at our source content"
Enter
Sleep 300ms

Type "cat samples/landing-page.html"
Enter
Sleep 2s

Type ""
Enter
Type "# Count words to estimate translation effort"
Enter
Sleep 300ms

Type "kapi word-count samples/landing-page.html"
Enter
Sleep 1500ms

Type ""
Enter
Type "# Pseudo-translate to test UI layout"
Enter
Sleep 300ms

Type "kapi pseudo-translate samples/landing-page.html --target-lang fr"
Enter
Sleep 1500ms

Type ""
Enter
Type "# View the pseudo-translated output"
Enter
Sleep 300ms

Type "cat out/landing-page.html"
Enter
Sleep 2s

Type ""
Enter
Type "# Open in Bowrain for visual editing, or use 'kapi serve' for web UI"
Enter
Sleep 1500ms
