# VHS Tape: kapi auth demo
# Demonstrates the CLI authentication commands

Output output/auth-login.webm
Output output/auth-login.gif

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# CLI Authentication"
Enter
Sleep 500ms

Type "kapi auth --help"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# Check current authentication status"
Enter
Sleep 500ms

Type "kapi auth status"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Login connects to a deployed bowrain-server"
Enter
Sleep 500ms

Type "kapi auth login --help"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# After login, 'auth status' shows your session info"
Enter
Sleep 500ms
Type "# → Server:  https://bowrain.example.com"
Enter
Type "# → User:    Jane Translator (jane@example.com)"
Enter
Type "# → Expiry:  2026-02-11T14:30:00Z"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Claim an anonymous local project into your server workspace"
Enter
Sleep 500ms

Type "kapi auth claim --help"
Enter
Sleep 3s

Type ""
Enter
Sleep 500ms

Type "# Remove stored credentials"
Enter
Sleep 500ms

Type "kapi auth logout"
Enter
Sleep 2s

Sleep 1s
