# VHS Tape: E2E walkthrough — kapi init (Quick start)
# Demonstrates Quick start mode for creating a project on the server

Output output/walkthrough-init.webm
Output output/walkthrough-init.gif

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# Step 1: Initialize a project with Quick start"
Enter
Sleep 500ms

Type "cd ../../examples/docusaurus-e2e"
Enter
Sleep 500ms

Type "rm -rf .kapi"
Enter
Sleep 300ms

Type "kapi init --quickstart --name 'Docusaurus E2E Demo' --source en --targets nb,fr --server http://localhost:8080"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# Check the generated config"
Enter
Sleep 500ms

Type "cat .kapi/config.yaml"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "kapi status"
Enter
Sleep 2s

Sleep 1s
