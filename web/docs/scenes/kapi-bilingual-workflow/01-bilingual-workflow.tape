# VHS tape — generated from web/docs/walkthroughs/kapi-bilingual-workflow.md
# Scene 1: bilingual-workflow (terminal). Do not edit by hand.

Output "01-bilingual-workflow.webm"

Set FontSize 16
Set Width 1000
Set Height 700
Set Theme "Dracula"
Set Padding 20

# Reset the fixture to a clean state so the recording is reproducible —
# kapi extract/merge mutate .kapi/ and src/locales/{fr-FR,de-DE}/.
Hide
Type "cd fixtures/bilingual-project && rm -rf .kapi out src/locales/fr-FR src/locales/de-DE && mkdir -p .kapi"
Enter
Sleep 300ms
Show

Type "# 1. One-time: seed the project TM from a corporate TMX."
Enter
Sleep 500ms
Type "kapi tm import corporate.tmx --file .kapi/tm.db --all-pairs"
Enter
Sleep 1500ms

Type "# 2. kapi extract — writes one XLIFF per target locale in the recipe."
Enter
Sleep 500ms
Type "kapi extract"
Enter
Sleep 2500ms

Type "ls out/"
Enter
Sleep 1500ms

Type "# 3. Simulate a translator: fill the <target>s."
Enter
Sleep 500ms
Type "./fake-translator.sh out/*.xliff"
Enter
Sleep 2000ms

Type "# 4. kapi merge — apply the returned XLIFFs back onto the source."
Enter
Sleep 500ms
Type "kapi merge -i out/"
Enter
Sleep 2500ms

Type "# 5. kapi tm audit — trace what this merge wrote into the TM."
Enter
Sleep 500ms
Type "kapi tm audit --file .kapi/tm.db --batch $(ls .kapi/extractions | head -1) --limit 10"
Enter
Sleep 2500ms

Type "cat src/locales/fr-FR/messages.json"
Enter
Sleep 2000ms

Type "# Source is localized; TM is warmer for the next extract."
Enter
Sleep 1500ms
