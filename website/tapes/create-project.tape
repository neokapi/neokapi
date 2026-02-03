# VHS Tape: End-to-end translation project creation
# Shows creating a .kaz project from source files

Output output/create-project.webm
Output output/create-project.gif

Set FontSize 15
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# Create a translation project with kapi"
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
Type "# Pack into a .kaz translation project"
Enter
Sleep 300ms

Type "kapi pack -i samples/landing-page.html --source-lang en --target-lang fr,de -o demo-project.kaz"
Enter
Sleep 1500ms

Type ""
Enter
Type "# Project created! Let's verify"
Enter
Sleep 300ms

Type "ls -lh demo-project.kaz"
Enter
Sleep 1s

Type ""
Enter
Type "# Now open in Bowrain to translate visually..."
Enter
Sleep 1500ms
