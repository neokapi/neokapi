# Page patterns and density

Kapi Desktop and the Bowrain platform show people words about words: rules,
terms, decisions and the sentences they were seen in. The content is text. The
page around it does not have to be. A screen that narrates its data in
sentences, explains each section under its heading and lays everything out as
one document reads as heavy even when it holds little. This file says which
shape a page takes, where explanation lives, and how a fact is shown, so that
density comes from the content and never from the chrome.

It rests on published guidance rather than on rules of our own. Where this
file and a reference disagree, the reference wins unless the exception is
named here. The two files beside it, [type-scale.md](type-scale.md) and
[judgement-colours.md](judgement-colours.md), fix text sizes and button
colours; this one fixes layout and copy density.

## What the references settle

| Question | Reference | What it says |
| --- | --- | --- |
| Which shapes a page can take | [Material 3 canonical layouts](https://m3.material.io/foundations/layout/canonical-layouts/overview) | List-detail, supporting pane and feed cover most screens. Pick one; do not invent a fifth. |
| How a desktop app arranges them | [Apple HIG: layout](https://developer.apple.com/design/human-interface-guidelines/layout), [split views](https://developer.apple.com/design/human-interface-guidelines/split-views), [lists and tables](https://developer.apple.com/design/human-interface-guidelines/lists-and-tables), [sidebars](https://developer.apple.com/design/human-interface-guidelines/sidebars) | A sidebar names places, a list shows items, a detail pane shows one. Rows are scannable and consistent. |
| How much to show at once | [Nielsen Norman Group: progressive disclosure](https://www.nngroup.com/articles/progressive-disclosure/) | Show the few things most people need first; the rest on request. |
| What a page must and must not do | [NN/g: ten usability heuristics](https://www.nngroup.com/articles/ten-usability-heuristics/) | Recognition rather than recall (6). Aesthetic and minimalist design (8): every extra unit of information competes with the relevant ones. |
| How people read a screen | [NN/g: how users read on the web](https://www.nngroup.com/articles/how-users-read-on-the-web/) | They scan. Lead with the point, one idea per paragraph, highlighted keywords, lists over prose. |
| How to group without labels | [NN/g: proximity](https://www.nngroup.com/articles/gestalt-proximity/), [common region](https://www.nngroup.com/articles/common-region/) | Space and a shared background group things. A heading over every group is a sign the spacing is wrong. |
| How to make one thing stand out | [Refactoring UI](https://www.refactoringui.com/) | De-emphasise the rest rather than enlarging the one. Labels are a last resort. Fewer borders. |
| Copy and content handling | [Vercel Web Interface Guidelines](https://github.com/vercel-labs/web-interface-guidelines) | Active voice, second person, numerals for counts, specific button labels, truncation and `min-w-0` for long content, tabular numbers in columns. |
| The component vocabulary | [shadcn/ui](https://ui.shadcn.com/) | The primitives in `src/components/ui` are these. A new pattern composes them before it adds one. |

One named exception. Vercel and the macOS HIG use Title Case for buttons and
headings. Our surfaces use sentence case, on the desktop, the platform and the
web alike, because they share copy with the docs and the CLI. Keep sentence
case.

## The four page shapes

Every screen is one of these. The pull request that adds or reshapes a screen
names which.

| Shape | What it is for | Regions |
| --- | --- | --- |
| Overview | The first screen of an app or a project: what is here and what needs you | A page header; a row of labelled counts; one or two lists of rows, each row leading to a place. Nothing on it is edited in place. |
| List and detail | Anything a person triages, browses or searches: suggestions, history, terms, content memory, the review queue | A list pane of one-line rows with selection, a detail pane for the selected item, and the item's actions in the detail pane's header. Keyboard moves the selection and fires the actions. |
| Settings and setup | Connections, sharing, bindings: a small number of facts, each with one action | Labelled rows, one control or button each. Where the state is a ladder, a stepper shows the rungs and the current one; the step's detail sits below it. |
| Reader | One document a person reads: the brief for a file, a rules file, a voice profile | A picker or title above, the document below, at reading width. Related documents are tabs, never stacked sections. |

A list-and-detail page keeps its list narrow and its rows one line tall. The
sentence an item was seen in, the file, who noticed it and when belong to the
detail pane, where there is room for all of them. Collapsing an item in place
is the document shape in disguise, and it leaves the list as long as before.

## Structure before sentences

A fact shown as a sentence has to be read; a fact shown in its place is seen.
These hold on every shape.

- **A number is a count with a label.** "kapi knows 61 rules across 4
  projects. 11 suggestions are advising." becomes four labelled counts in a
  row: projects 4, rules 61, suggestions 11, needs you 1. Numbers in a column
  take `tabular-nums`.
- **A status is a badge or an icon with a word.** `StatusBadge` for a ladder;
  an icon beside a word for a connection. A clause ("is not yet set up",
  "is working there now") is a status written out.
- **A row has one primary line and at most one meta line.** The meta line
  carries at most three facts, separated by a middle dot. A fourth fact goes
  to the detail pane or a tooltip.
- **A fact appears once per screen.** A project's name, its count of new
  items and its path are each shown in one place.
- **A path shows its last segment.** The full path is the tooltip, or a line
  in the detail pane. A path inside a sentence pushes the sentence to two
  lines and reads as a value, not as prose.
- **Who did something is a mark and a name.** An assistant host, a person and
  a sync each get an avatar or icon, so a reader recognises the actor before
  reading the name.

## Explanation lives in four places

A paragraph under a heading that tells the reader what the section is for
is the first thing to remove from a screen. It is read once and seen every
time. Explanation belongs in one of these, in this order of preference:

1. **The empty state.** When a section has nothing, `EmptyState` says what
   would appear and offers the one action that fills it. This is the only
   place a section explains itself in full.
2. **An affordance on the heading.** A tooltip or popover behind an
   information mark, for a reader who asks.
3. **The first-run card.** "Start here" on the overview, dismissed once.
4. **The docs.** A link, when the explanation runs past a sentence.

The line that names a page's command-line equivalent is a mono chip in the
page header's right-hand slot, as the Assistant page shows `kapi init`. It
appears once per page and never as a caption under a section.

## Hierarchy

- A page has one title, through `PageHeader`. Groups inside it take
  `SectionHeading` only when spacing and background do not separate them on
  their own. Three sections in view at once is the ceiling; the rest scroll.
- A page has one `default` button, the act it exists for. Row actions are
  `outline` or `ghost`. See [judgement-colours.md](judgement-colours.md).
- Emphasis comes from weight and colour, not from more words. The primary
  line is body weight in the foreground colour; everything else on the row is
  the caption step in the muted colour.
- Space and background separate regions before a border does. A border marks
  an interactive or selected region. A card never holds another card.
- Spacing steps are multiples of four pixels, taken from the scale the
  components already use. A list row is 40 to 48 pixels tall. Prose reads at
  65 to 75 characters a line; lists and tables take the pane.
- A keyboard shortcut shows as `kbd` on hover or focus, not permanently on
  every row.

## Copy

- Sentence case, second person, active voice, numerals for counts.
- A button names its act: Keep, Change, Drop, Connect, Sync now. Never
  Continue or OK.
- An error names what happened and the next step.
- The vocabulary and register are those of
  [brand-communication.md](../../../docs/internals/brand-communication.md).

## Checklist for a page pull request

- The page's shape is named in the description.
- No explanatory paragraph sits under a heading; each explanation is in one
  of the four places above.
- Every number has a label and none is inside a generated sentence.
- Each fact appears once on the screen.
- One `default` button.
- No meta line carries more than three facts.
- Paths show their last segment, full on hover.
- The command-line equivalent is in the header slot and nowhere else.
- Empty states use `EmptyState` with one action.
- Stories cover empty, one item, many items and long content, in light and
  dark.
