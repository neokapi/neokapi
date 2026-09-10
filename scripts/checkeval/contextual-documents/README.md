# Contextual document development cases

These cases ask whether a coherent guide preserves the product behavior and
actions its reader needs. Each family includes excerpts from repository
documentation, a stated destination and task, and separately authored supported
and faulty candidates. The candidates are adaptations for evaluation, not
published product documentation or naturally occurring user errors.

The source text is copied exactly from the recorded line range of the recorded
Git blob. Its repository path and blob identity establish provenance; they do
not establish that the documentation itself is an independently verified product
specification. Changes to the repository do not silently change this evidence.

Each family declares `requirements` with an `id`, a reader-action `description`
and `source_ids` that refer to its evidence. These requirements define the task's
necessary actions and decisions before any candidate is assessed. They are
shared unchanged across the family's variants and belong in subject inputs;
they contain no candidate quotations, error locations or expected judgments.

The granularity follows the stated task: destination checking includes an
explicit post-save file check, not merely a discussion of file-check options;
deliberate overrides require an account of their voice and terminology effects.
Concurrent editing requires preparation, preview, interpretation of blocked
writes, recovery and verification. A release handoff requires inspectable
project evidence, measured scope, limits of assurance and failure handling.
Source details such as a numeric exit code, optional similarity analysis or an
individual release gate are not separate mandatory sections. A requirement can
be satisfied by supported paraphrases or by guidance spread across sections.
Additional claims remain subject to evidence checking even when they are not
needed to meet a requirement. The requirements are authored development inputs,
not an independent validation of completeness.

The labels are provisional author labels. An independent review must check both
the alleged errors and the supported candidates before model runs. Expected
issues identify meaning and missing action guidance, rather than preferred
wording. Other supported paraphrases can be valid. The planted issue inventory
is not an exhaustive judgment of every possible problem in a candidate.

Keep all variants of a family in the development partition. Put each candidate
in a separate fresh subject context. The evaluator retains this directory:
`expected`, variant names and the relationship between variants must be removed
from subject inputs. File and source identity checks validate preparation, not
semantic quality or analyzer accuracy. No inference calls are part of these
fixtures.
