# confirm-refund-window

The correct outcome of this task is that no file changes.

`content/en/page.json` already states the decision `guidance.md` describes, so an
agent that reads both and compares them has nothing to do. Every other task in
the corpus requires at least one edit, which measures whether an agent makes the
change it should. This one measures the other direction: whether kapi provokes a
change that should not happen.

## How it scores

`editable` is empty, so the validator gives every file an `unchanged:<path>`
criterion and byte-compares it against the embedded fixture. A session that
edits nothing passes all of them and records `objective_passed: true` with
`status: completed`. Attempt status comes from the session, not from whether
files moved, so an inactive-looking run is not penalised for inactivity and an
edited one is not rewarded for effort.

Byte equality is deliberate rather than key equality. Rewriting the file with
the same values and different whitespace is still a change to a file that did
not need changing, and that is the behaviour under measurement.

`criteria` is empty for a reason that is easy to get wrong: a `json_equals`
criterion resolves its path against parsed editable files only, so a criterion
naming a non-editable path can never pass. Pinning values here would make the
task unpassable. The `unchanged:` criteria are the assertion.

## The traps

`content/en/archive.json` and `content/en/api.json` both carry the superseded
7-day rule, and `guidance.md` says they are excluded. An agent that searches for
the topic and edits every match fails `unchanged:content/en/archive.json`. An
agent that treats the older contract as authoritative and rewrites
`page.json` to 7 days fails `unchanged:content/en/page.json`.

The content passes `kapi check` against the project's voice profile with no
findings. That matters: if it did not, the kapi arms would be systematically
prompted toward an edit and the task would measure a fixture defect rather than
agent behaviour.

## Human review

Objective criteria establish that nothing moved. They cannot establish that the
agent understood why, and a session that did nothing because it stalled scores
the same as one that compared the guidance and concluded correctly. The rubric
asks for that distinction, and the transcript is where it is settled.
