# Judgement colours

Kapi Desktop and the Bowrain platform show the same acts to the same people.
Approving a translation is one act, whichever surface the reviewer happens to be
on, so it is one colour. This file says which colour carries which judgement.
The values live in `src/styles/semantic-colors.css`, which both apps import
alongside their own palette.

## The four button variants that carry a judgement

| Act | Variant | Examples |
| --- | --- | --- |
| Accept and move forward | `success` | Approve, Sign off, Publish, Merge, Accept suggestion |
| Hand back to a person | `warning` | Send back, Request changes, Needs attention, Unblock |
| Remove or undo something | `destructive` | Reject, Delete, Discard, Revoke, Cancel run |
| The page's main forward action | `default` | Run, Save, Create, Continue |

`default` is the primary colour, and a page gets one of it. It marks the single
thing the page exists to do, so a second primary button on the same view means
one of the two is really a `secondary`, a `success` or a `ghost`.

`success` and `warning` are for judgements a person makes, not for reporting
state. A green Approve button says "approving is what you do here"; a green
badge saying "signed off" reports what already happened. Both are green, and
only the first is a button.

Everything else stays on `secondary`, `outline` and `ghost`. A button with no
judgement in it does not need a hue to say so.

## Status, and the two ladders

`StatusBadge` draws both ladders on one scale, so a reader who has learnt one
can read the other:

| Rung | Content ladder | Source ladder | Tone |
| --- | --- | --- | --- |
| Below the ladder | `not-started` | | muted |
| Bottom | `draft` | `authored` | muted |
| Middle | `translated` | | neutral |
| Earned by a check or a review | `reviewed` | `checked` | soft green |
| Signed for by a person | `signed-off` | `approved` | filled green |

`blocked` and `attention` sit on neither ladder and take warning, because
something is waiting for a person. The three-rung source ladder skips the
neutral stop rather than compressing the scale, which keeps `checked` and
`reviewed` the same colour and `approved` and `signed-off` the same colour.

Statuses are the wire values, hyphen and all (`signed-off`, never
`signed_off`), so a caller hands the badge whatever the API returned.

## Coordinate axes

`CoordinateChip` gives each axis of a context coordinate a hue and an icon:
product is warm red, channel is blue, brand is violet, language is teal. An axis
a recipe invented takes the neutral tint under a generic mark. The hues come
from the same prism ramp as the entity marks, so a chip rail and a marked-up
source paragraph belong to one drawing.

The hue is a shorthand, so the axis name is always spelt out in the chip's
tooltip and accessible name ("Channel: reference"). Nobody has to learn the
colours to read the page.

## Languages

A language is shown by name with its tag beside it in muted monospace, resolved
in the language the reader is reading: "French (France) fr-FR", or
"fransk (Frankrike) fr-FR" for a Norwegian reader. `LocaleLabel` is that
rendering, and `formatLocale` is the same resolution outside React.

Where a name does not fit, `compact` draws the tag alone and puts the name in
the tooltip. The tag itself is rendered exactly as it was given: case carries
meaning in BCP 47, so `zh-Hant` and `sr-Latn-RS` never take an `uppercase`
class.

## Context is neutral; judgement carries the severity

A review surface draws two different things. The layers above the verdict say
what the model was told: the coordinates a unit sits at, the voice profile in
force, the term rules bound at the point, what the content memory already holds.
The verdict says what was found in this unit.

Only the verdict takes a severity colour. Term rules on a Point card render in a
neutral chip whatever their `severity`, with the bite ("blocks approval" /
"warns only") in the tooltip and a do-not-translate rule marked by a lock rather
than by a fill. A red chip there reads as a defect, and a rule resolved from a
terms store carries no severity at all, so painting by severity turned a whole
card into a wall of red pairs a reviewer had nothing to do about. The findings
in `ReviewJudgement.Findings` and `AIFindings` are where a term rule turns red,
because a finding says this unit broke one.

The same holds for the other context a review page shows: a prior approval whose
governing context has moved is marked in muted ink, and the wording currently in
force under an AI proposal is drawn on a neutral ground rather than as the
rejected half of a diff.

## What is out of scope here

Findings severity has its own scale. A voice finding, a term violation and a tag
error are graded by how much they matter, which is a different question from
what a person is being asked to do, and mixing the two would put a red Reject
button and a red error count on the same row meaning different things.

## Contrast

Every pair in `semantic-colors.css` clears WCAG AA (4.5:1) for text at its
intended use, in light and dark. The green is darker than a green picked by eye,
because green at a comfortable lightness carries too much luminance to hold
white text, and the platform reads `--success` as ink on the page about as often
as it fills a button. Check a new pair before adding it: an amber fill wants
dark ink, a green fill wants white ink in light mode and dark ink in dark mode.
