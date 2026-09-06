# Container markers

A list item whose only content is a dropped inline construct leaves a bare
block marker behind, which the rebuild path escapes so the item survives.

- <span>#</span> after a span
- \# an escaped hash
- \> an escaped quote marker
- \- an escaped bullet

> \# an escaped hash inside a quote
> and a second line

1. \1. an escaped ordinal
2. plain text
