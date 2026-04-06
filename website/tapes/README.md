# Kapi CLI Demo Videos

VHS tape files for generating standalone `kapi` CLI demo videos.

For Bowrain CLI recordings, see [`bowrain/e2e/tapes/`](../../bowrain/e2e/tapes/).

## Prerequisites

```bash
brew install charmbracelet/tap/vhs
```

## Generating Videos

```bash
# Generate all kapi demos
./generate.sh

# Or run individual tapes
vhs overview.tape
vhs word-count.tape
vhs pseudo-translate.tape
```

## Output

Videos are generated as WebM files in:

- `website/tapes/output/` — raw output
- `website/static/video/kapi/` — copied for docs

## Tape Files

| Tape                    | Description                    |
| ----------------------- | ------------------------------ |
| `overview.tape`         | Quick tour of kapi commands    |
| `word-count.tape`       | Word counting for estimation   |
| `pseudo-translate.tape` | Pseudo-translation for testing |

## Sample Files

The `samples/` directory contains example files used in demos:

- `messages.json` — sample JSON localization file
