# CLI Demo Videos

This folder contains [VHS](https://github.com/charmbracelet/vhs) tape files for generating CLI demo videos.

## Prerequisites

```bash
brew install charmbracelet/tap/vhs
```

## Generating Videos

VHS requires a local terminal with TTY access (won't work in SSH or CI environments).

```bash
# Generate all demos
./generate.sh

# Or run individual tapes
vhs overview.tape
vhs convert.tape
vhs word-count.tape
vhs pseudo-translate.tape
```

## Output

Videos are generated in two formats:
- **WebM** - For web embedding (smaller, better quality)
- **GIF** - For README/GitHub embeds

Output files go to:
- `website/tapes/output/` - Raw output
- `website/static/video/cli/` - Copied for docs

## Tape Files

| Tape | Description |
|------|-------------|
| `overview.tape` | Quick tour of kapi commands |
| `convert.tape` | Format conversion demo |
| `word-count.tape` | Word counting for estimation |
| `pseudo-translate.tape` | Pseudo-translation for testing |

## Sample Files

The `samples/` directory contains example files used in demos:
- `messages.json` - Sample JSON localization file

## Customizing

See [VHS documentation](https://github.com/charmbracelet/vhs) for tape syntax.

Common settings:
```tape
Set FontSize 16
Set Width 900
Set Height 500
Set Theme "Dracula"
```

Available themes: `vhs themes`
