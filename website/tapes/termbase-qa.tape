# VHS Tape: kapi termbase QA demo
# Demonstrates terminology QA workflow: import glossary, look up terms,
# run QA check with termbase enforcement.

Output output/termbase-qa.webm

Set FontSize 16
Set Width 1000
Set Height 650
Set Theme "Dracula"
Set Padding 20

Type "# Import a glossary into a named termbase"
Enter
Sleep 500ms

Type "cat samples/glossary.csv"
Enter
Sleep 1500ms

Type "kapi termbase import samples/glossary.csv --name product-terms --format csv -s en -t fr"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Check termbase statistics"
Enter
Sleep 500ms

Type "kapi termbase stats --name product-terms"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Look up a specific term"
Enter
Sleep 500ms

Type "kapi termbase lookup password --name product-terms -s en -t fr"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Search across all terms"
Enter
Sleep 500ms

Type "kapi termbase search encrypt --name product-terms -s en"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Run QA with terminology enforcement"
Enter
Sleep 500ms

Type "kapi flow run pseudo-translate -i samples/messages_en.json -o out/pseudo_fr.json --target-lang fr"
Enter
Sleep 1500ms

Type "kapi flow run qa-check -i out/pseudo_fr.json -o out/qa_report.json --source-lang en --target-lang fr --termbase product-terms"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Terminology violations are flagged in the output"
Enter
Sleep 1s
