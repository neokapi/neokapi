# VHS Tape: create-project demo
# Shows creating a localization project from scratch

Output output/create-project.webm

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# Let's create a simple localization project"
Enter
Sleep 1s

Type "cat samples/landing-page.html"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# Pseudo-translate to French"
Enter
Sleep 500ms

Type "kapi pseudo-translate samples/landing-page.html --target-lang fr"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "cat out/landing-page.html"
Enter
Sleep 3s

Sleep 1s
