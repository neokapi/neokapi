# VHS Tape: brain init demo
# Demonstrates the interactive project initialization

Output output/kapi-init.webm

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# Initialize a new Brain project"
Enter
Sleep 500ms

Type "brain init --help"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# Non-interactive: create a project with flags"
Enter
Sleep 500ms

Type "brain init --name 'My App' --source en-US --targets fr-FR,de-DE"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Check the generated project config"
Enter
Sleep 500ms

Type "cat .brain/config.yaml"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# View project status"
Enter
Sleep 500ms

Type "brain status"
Enter
Sleep 2s

Sleep 1s
