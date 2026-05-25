---
sidebar_position: 5
title: JavaScript Script Step
description: The script step lets you run custom ES5 JavaScript on each Part flowing through a neokapi pipeline — using the goja pure-Go runtime, with access to emit(), skip(), and log() for controlling Part flow.
keywords: [script step, JavaScript, ES5, goja, pipeline, custom processing, Part, neokapi]
---

# JavaScript Script Step

The script step lets you run custom JavaScript (ES5) on each Part flowing through the pipeline. It uses the [goja](https://github.com/dop251/goja) ES5 runtime -- a pure Go JavaScript engine with no CGo dependency.

## How it works

The script tool receives each Part one at a time. For each Part, your script runs with access to a `part` object and three control functions: `emit()`, `skip()`, and `log()`. If your script calls neither `emit()` nor `skip()`, the Part passes through unchanged.

## The JavaScript API

### The part object

```javascript
part.type; // "block", "data", "media", "layer-start", "layer-end",
// "group-start", "group-end"
```

For block parts, the `block` property provides access to translatable content:

```javascript
part.block.id; // Block ID string
part.block.translatable; // boolean

// Source runs (flat array)
part.block.source[0].content.text; // text of the first run

// Target runs by locale (object)
part.block.targets["fr"][0].content.text; // French target's first run text
```

### emit(part)

Emit a modified (or new) Part to the output channel. If you call `emit()`, the original Part is **not** forwarded automatically -- only what you emit reaches downstream tools.

```javascript
// Modify a target translation and emit
if (part.block.targets["fr"]) {
  var seg = part.block.targets["fr"][0];
  seg.content.text = seg.content.text.toUpperCase();
}
emit(part);
```

By default the script may read the source but only **target** edits are read
back — the source is read-only (see [Configuration reference](#configuration-reference)).

### skip()

Drop the current Part entirely. It will not reach downstream tools or the writer.

```javascript
if (part.type === "block" && part.block.source[0].content.text === "") {
  skip();
}
```

### log(message)

Write a message to stderr for debugging.

```javascript
log("Processing block: " + part.block.id);
```

### Control flow summary

| Script behavior                | Result                                        |
| ------------------------------ | --------------------------------------------- |
| No `emit()` or `skip()` called | Part passes through unchanged                 |
| `emit(part)` called            | Only emitted parts are forwarded              |
| `skip()` called                | Part is dropped                               |
| `emit()` called multiple times | All emitted parts are forwarded (one-to-many) |

## CLI usage

### Inline code

```bash
kapi script -i input.xliff --code 'if (part.type === "block") {
  var text = part.block.source[0].content.text;
  if (text.length > 100) { skip(); }
}'
```

### Script file

```bash
kapi script -i input.xliff --script-file filter.js
```

Where `filter.js` contains:

```javascript
if (part.type === "block") {
  var text = part.block.source[0].content.text;
  if (text.length <= 5) {
    skip();
  }
}
```

## YAML flow usage

Use the script step inline in a flow definition:

```yaml
steps:
  - tool: script
    label: Filter short segments
    config:
      code: |
        if (part.type === 'block') {
          var text = part.block.source[0].content.text;
          if (text.length < 3) {
            skip();
          }
        }

  - tool: pseudo-translate
    config:
      targetLocale: fr
```

Or reference an external file:

```yaml
steps:
  - tool: script
    config:
      scriptFile: ./scripts/filter.js

  - tool: pseudo-translate
    config:
      targetLocale: fr
```

## Examples

### Filter by source text length

Skip blocks where the source text is shorter than a threshold:

```javascript
if (part.type === "block") {
  var text = part.block.source[0].content.text;
  if (text.length < 10) {
    skip();
  }
}
```

### Modify target text

Append a marker to all French translations:

```javascript
if (part.type === "block" && part.block.targets["fr"]) {
  var seg = part.block.targets["fr"][0];
  seg.content.text = seg.content.text + " [REVIEW]";
  emit(part);
}
```

### Conditional routing

Only pass translatable blocks through to downstream tools:

```javascript
if (part.type !== "block") {
  // Let structural parts (layers, data) pass through
  emit(part);
} else if (part.block.translatable) {
  emit(part);
} else {
  skip();
}
```

### Transform source text

Normalize whitespace in the source before translation. Source edits are
**ignored by default** — the source is read-only to the script (immutability
contract). Opt in with `allowSourceMutation: true`, and place the step in a
flow's [source-transform stage](/contribute/flow-authoring) so the model is
settled before any annotation or translation runs:

```yaml
source_transforms:
  - tool: script
    config:
      allowSourceMutation: true
      code: |
        if (part.type === 'block') {
          var text = part.block.source[0].content.text;
          text = text.replace(/\s+/g, ' ').replace(/^\s+|\s+$/g, '');
          part.block.source[0].content.text = text;
          emit(part);
        }
```

### Log and pass through

Inspect the pipeline without changing anything:

```javascript
if (part.type === "block") {
  log("Block " + part.block.id + ": " + part.block.source[0].content.text);
}
// No emit() or skip() -- part passes through unchanged
```

## Configuration reference

| Property              | Type    | Description                                                                       |
| --------------------- | ------- | --------------------------------------------------------------------------------- |
| `source`              | string  | Mode selector: `inline` (default) or `file`                                       |
| `code`                | string  | Inline JavaScript code (ES5)                                                      |
| `scriptFile`          | string  | Path to a `.js` file                                                              |
| `allowSourceMutation` | boolean | Permit the script to modify the source text. Off by default — the source is read-only and source edits are ignored unless this is set. |

Provide either `code` or `scriptFile`. The optional `source` field selects the
mode explicitly (`inline` or `file`) for UI and validation; when omitted, the
mode is inferred from whichever of `code`/`scriptFile` is set.

## Notes

- The runtime is ES5 only (no `let`, `const`, arrow functions, or template literals). Use `var` for variable declarations.
- Each tool instance gets its own goja runtime, so there is no shared state between parallel pipeline branches.
- The script runs synchronously for each Part. Long-running scripts will block the pipeline.
- Target text edits on block parts are read back. Source edits are read back only when `allowSourceMutation: true`; otherwise the source is read-only. Changes to other Part types are not persisted.
