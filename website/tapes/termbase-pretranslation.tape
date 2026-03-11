# VHS Tape: kapi termbase pre-translation demo
# Demonstrates the full pre-translation pipeline:
# import TM + termbase → TM leverage → pseudo-translate remaining → QA check.

Output output/termbase-pretranslation.webm

Set FontSize 16
Set Width 1000
Set Height 650
Set Theme "Dracula"
Set Padding 20

Type "# Set up language assets: termbase + translation memory"
Enter
Sleep 500ms

Type "kapi termbase import samples/glossary.csv --name product-terms --format csv -s en -t fr --header"
Enter
Sleep 1500ms

Type "kapi tm import samples/project.tmx --name project-tm -s en -t fr"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Source file to translate"
Enter
Sleep 500ms

Type "cat samples/messages_en.json"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Step 1: Leverage translation memory"
Enter
Sleep 500ms

Type "kapi flow run tm-leverage -i samples/messages_en.json -o out/step1_tm.json --source-lang en --target-lang fr --tm project-tm"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Step 2: Translate remaining segments"
Enter
Sleep 500ms

Type "kapi flow run pseudo-translate -i out/step1_tm.json -o out/step2_translated.json --target-lang fr"
Enter
Sleep 1500ms

Type ""
Enter
Sleep 500ms

Type "# Step 3: Verify terminology compliance"
Enter
Sleep 500ms

Type "kapi flow run qa-check -i out/step2_translated.json -o out/step3_qa.json --source-lang en --target-lang fr --termbase product-terms"
Enter
Sleep 2s

Type ""
Enter
Sleep 500ms

Type "# Full pipeline complete: TM leverage → translate → QA"
Enter
Sleep 1s
