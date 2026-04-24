# VHS Tape: kapi extract → merge bilingual round-trip (AD-017, issue #421)
#
# Runs the end-to-end workflow narrated in
# website/docs/kapi-cli/bilingual-workflow.md:
#   1. Seed the project TM from a TMX corpus (optional).
#   2. kapi extract — emit XLIFF per target locale with TM pre-fill.
#   3. Simulate a translator completing the XLIFF (scripted here).
#   4. kapi merge — apply the translator's return back onto the source,
#      absorbing into the TM with batch provenance.
#   5. kapi tm audit --batch <id> — show what merge contributed.
#
# To regenerate, ensure samples/ holds a clean fixture:
#   - samples/bilingual-project/app.kapi
#   - samples/bilingual-project/src/locales/en/messages.json
# and a pre-seeded TMX at samples/bilingual-project/corporate.tmx.
#
# Invoked via website/tapes/generate.sh as part of `make kapi-recordings`.

Output output/bilingual-workflow.webm

Set FontSize 16
Set Width 1000
Set Height 700
Set Theme "Dracula"
Set Padding 20

Type "cd samples/bilingual-project"
Enter
Sleep 300ms

Type "# 1. One-time: seed the project TM from a corporate TMX."
Enter
Sleep 500ms
Type "kapi tm import corporate.tmx -p app.kapi"
Enter
Sleep 1500ms

Type "# 2. kapi extract — writes one XLIFF per target locale in the recipe."
Enter
Sleep 500ms
Type "kapi extract"
Enter
Sleep 2000ms

Type "ls out/"
Enter
Sleep 1500ms

Type "# 3. Simulate a translator: fill the <target>s."
Enter
Sleep 500ms
Type "./fake-translator.sh out/*.xliff"
Enter
Sleep 1500ms

Type "# 4. kapi merge — apply the returned XLIFFs back onto the source."
Enter
Sleep 500ms
Type "kapi merge -i out/"
Enter
Sleep 2000ms

Type "# 5. kapi tm audit — trace what this merge wrote into the TM."
Enter
Sleep 500ms
Type "kapi tm audit --batch $(ls .kapi/extractions | head -1) --limit 10"
Enter
Sleep 2500ms

Type "# Source is localized; TM is warmer for the next extract."
Enter
Sleep 1500ms
