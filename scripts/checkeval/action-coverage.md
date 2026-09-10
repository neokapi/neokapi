# Action coverage development comparison

The [corpus](action-coverage.json) contains fictional museum, event and editorial
procedures. All policies and guides are authored synthetic material. They are
neither production facts nor an independent benchmark. Each family has a shared
reader task, evidence and requirements, with three guides of approximately
180–250 words. A single passage provides a direct instruction, expresses the
same action through a clear prerequisite or handoff condition, or describes
available features while omitting the action. Supported indirect wording is a
valid way to meet a requirement.

The declared schedule selects the museum direct instruction, event indirect
instruction and editorial omission. Each receives an ordinary review and a
requirement-coverage review from the same fixed Sonnet 5 high configuration.
Both receive identical candidate and task data. Protocol order alternates
between families. The other six variants remain unrun development controls.
This small selection examines distinct coverage judgments; it does not estimate
population precision, distinguish every domain effect, or show integration
benefit.

Prepare a fresh directory without calling a model:

```sh
python3 scripts/checkeval/prepare_actions.py --out harness/out/action-coverage
```

Open `harness/out/action-coverage/review.html` in a browser to read the context,
sources and all variants. The `subject` subdirectory contains only the three
scheduled inputs, shared instructions and the six-session runner manifest.
Labels, selection details, unscheduled guides and full source provenance remain
outside it. Input IDs conceal variant names; sources receive neutral IDs. The
preparer records hashes of the corpus and its own code, copies the complete
source material and labels, and refuses to overwrite an existing output.

The existing meaning runner accepts the prepared `subject` directory through
`-meaning-inputs`. Its explicit live switch, signed-in subscription requirement,
no-tool restriction and persistent six-attempt ceiling apply. Preparation does
not launch reviews. Freeze the labels and obtain a separate pre-run assessment
before reading subject results. Keep that assessment separate from the authored
labels and identify agent assessment as such. Changes after results are read
require a new development batch with its own provenance.

Validate preparation offline:

```sh
python3 -m unittest discover -s scripts/checkeval -p 'test_prepare_actions.py'
```

These checks verify input shape, source references, bounded scheduling,
reproducibility and separation of labels. They cannot establish the correctness
of the semantic judgments. Assess conflicts independently of requirement
coverage; a feature description can be factually supported while omitting an
action the reader needs.
