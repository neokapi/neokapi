# VHS Tape: kapi auth login demo
# Demonstrates the CLI authentication flow

Output output/auth-login.webm
Output output/auth-login.gif

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20

Type "# Authenticate the CLI against a gokapi server"
Enter
Sleep 500ms

Type "kapi auth login --server http://localhost:8080"
Enter
Sleep 1s

Type "# → Open http://localhost:8080/auth/device and enter code: ABCD-1234"
Enter
Sleep 2s

Type "# → Waiting for authorization..."
Enter
Sleep 1500ms

Type "# → Logged in as translator@example.com"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Check authentication status"
Enter
Sleep 500ms

Type "kapi auth status"
Enter
Sleep 1s

Type "# → Server:  http://localhost:8080"
Enter
Type "# → User:    translator@example.com"
Enter
Type "# → Name:    Jane Translator"
Enter
Type "# → Expires: 2026-02-11 14:30:00"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Logout when done"
Enter
Sleep 500ms

Type "kapi auth logout"
Enter
Sleep 1s

Type "# → Logged out from http://localhost:8080"
Enter
Sleep 2s

Sleep 1s
