# VHS tape — generated from website/walkthroughs/kapi-terminology-pretranslation.md
# Scene 1: termbase-pretranslation (terminal). Do not edit by hand.

Output "01-termbase-pretranslation.webm"

Set FontSize 16
Set Width 1000
Set Height 650
Set Theme "Dracula"
Set Padding 20

Type "# Set up language assets: termbase + translation memory"
Enter
Sleep 500ms

Type "kapi termbase import fixtures/glossary.csv --name product-terms --format csv -s en -t fr --header"
Enter
Sleep 1500ms

Type "kapi tm import fixtures/project.tmx --name project-tm -s en -t fr"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Source file to translate"
Enter
Sleep 500ms

Type "cat fixtures/messages_en.json"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Step 1: Leverage translation memory"
Enter
Sleep 500ms

Type "kapi tm-leverage -i fixtures/messages_en.json -o out/step1_tm.json --source-lang en --target-lang fr --tm project-tm"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Step 2: Translate remaining segments"
Enter
Sleep 500ms

Type "kapi pseudo-translate -i out/step1_tm.json -o out/step2_translated.json --target-lang fr"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Step 3: Verify terminology compliance"
Enter
Sleep 500ms

Type "kapi qa-check -i out/step2_translated.json -o out/step3_qa.json --source-lang en --target-lang fr --termbase product-terms"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Full pipeline complete: TM leverage → translate → QA"
Enter
Sleep 1s
