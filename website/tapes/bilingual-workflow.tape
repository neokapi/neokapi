# VHS Tape: kapi extract → merge bilingual round-trip (AD-017, issue #421)
#
# Runs the end-to-end workflow narrated in
# website/docs/kapi-cli/bilingual-workflow.md:
#   1. Seed the project TM from a TMX corpus.
#   2. kapi extract — emit XLIFF per target locale with TM pre-fill.
#   3. Simulate a translator completing each XLIFF (fake-translator.sh).
#   4. kapi merge — apply the translator's return back onto the source,
#      absorbing into the TM with batch provenance.
#   5. kapi tm audit --batch <id> — show what merge contributed.
#
# Fixture under samples/bilingual-project/:
#   - app.kapi              recipe: en-US → {fr-FR, de-DE}, json content
#   - corporate.tmx         5 TUs; 4 match messages.json for exact leverage
#   - src/locales/en/messages.json (5 keys, one multi-sentence)
#   - fake-translator.sh    inline Python that fills <target> per locale
#
# Invoked via website/tapes/generate.sh as part of `make kapi-recordings`.

Output output/bilingual-workflow.webm

Set FontSize 16
Set Width 1000
Set Height 700
Set Theme "Dracula"
Set Padding 20

# Reset the fixture to a clean state so the recording is reproducible —
# kapi extract/merge mutate .kapi/ and src/locales/{fr-FR,de-DE}/.
Hide
Type "cd samples/bilingual-project && rm -rf .kapi out src/locales/fr-FR src/locales/de-DE && mkdir -p .kapi"
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
