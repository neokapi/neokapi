---
sidebar_position: 5
title: JavaScript Script Step
description: The script step lets you run custom ES5 JavaScript on each Part flowing through a neokapi pipeline, using the goja pure-Go runtime, with access to emit(), skip(), and log() for controlling Part flow.
keywords: [script step, JavaScript, ES5, goja, pipeline, custom processing, Part, neokapi]
---

# JavaScript Script Step

The `script` tool runs your ES5 JavaScript on each Part that passes through a
flow. Use it to filter, route or edit Parts without writing a Go tool.

The runtime is [goja](https://github.com/dop251/goja), a pure Go ES5 engine with
no cgo dependency.

## How it works

The script tool receives each Part one at a time. For each Part, your script runs with access to a `part` object and three control functions: `emit()`, `skip()`, and `log()`. If your script calls neither `emit()` nor `skip()`, the Part passes through unchanged.

You can write your logic in either of two equivalent forms: as **top-level code** against the implicit `part` global (used throughout the examples below), or as a **`function process(part)`** that the tool calls once per Part (see [Function form](#function-form)). Both have identical capabilities; the function form adds a return-value convenience.

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

Emit a modified (or new) Part to the output channel. Once you call `emit()`, only what you emit reaches downstream tools, and the
original Part is dropped.

```javascript
// Modify a target translation and emit
if (part.block.targets["fr"]) {
  var seg = part.block.targets["fr"][0];
  seg.content.text = seg.content.text.toUpperCase();
}
emit(part);
```

By default the script may read the source but only **target** edits are read
back, because the source is read-only (see [Configuration reference](#configuration-reference)).

### skip()

Drop the current Part. Downstream tools and the writer never see it.

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

### Function form

Instead of writing top-level code against the implicit `part` global, you can define a `process` function. The tool calls it once per Part, passing the part as its argument:

```javascript
function process(part) {
  if (part.type === "block" && part.block.targets["fr"]) {
    var seg = part.block.targets["fr"][0];
    seg.content.text = seg.content.text.toUpperCase();
  }
  return part; // emit the (modified) part
}
```

Inside `process`, `emit()`, `skip()`, and `log()` behave exactly as in the implicit-globals form. As a convenience, the function's **return value** is also honored, but only when you have _not_ already called `emit()` or `skip()` (an explicit call always wins):

| `process(part)` returns | Result                                          |
| ----------------------- | ----------------------------------------------- |
| a part object           | that part is emitted                            |
| an array of parts       | all are emitted (one-to-many)                   |
| `null`                  | the part is dropped (equivalent to `skip()`)    |
| nothing (`undefined`)   | the part passes through unchanged               |

The function form works identically with `--code`, `--script-file`, and the YAML `code`/`script_file` config. It reads naturally for transforms that compute and return a result; the implicit-globals form is terser for simple filters. Source edits still require `allow_source_mutation` (see [Configuration reference](#configuration-reference)) in either form.

## CLI usage

### Inline code

```bash
kapi exec script -i input.xliff --code 'if (part.type === "block") {
  var text = part.block.source[0].content.text;
  if (text.length > 100) { skip(); }
}'
```

### Script file

```bash
kapi exec script -i input.xliff --script-file filter.js
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
      target_locale: fr
```

Or reference an external file:

```yaml
steps:
  - tool: script
    config:
      script_file: ./scripts/filter.js

  - tool: pseudo-translate
    config:
      target_locale: fr
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
**ignored by default**, because the source is read-only to the script
(immutability contract). Opt in with `allow_source_mutation: true`, and place the
step ahead of the steps that should observe the rewritten source, typically first (see
[flow authoring](/contribute/flow-authoring)):

```yaml
steps:
  - tool: script
    config:
      allow_source_mutation: true
      code: |
        if (part.type === 'block') {
          var text = part.block.source[0].content.text;
          text = text.replace(/\s+/g, ' ').replace(/^\s+|\s+$/g, '');
          part.block.source[0].content.text = text;
          emit(part);
        }
  - tool: translate
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
| `script_file`          | string  | Path to a `.js` file                                                              |
| `allow_source_mutation` | boolean | Permit the script to modify the source text. Off by default: the source is read-only and source edits are ignored unless this is set. |

Provide either `code` or `script_file`. The optional `source` field selects the
mode explicitly (`inline` or `file`) for UI and validation; when omitted, the
mode is inferred from whichever of `code`/`script_file` is set.

## Notes

- The runtime is ES5 only (no `let`, `const`, arrow functions, or template literals). Use `var` for variable declarations.
- Each tool instance gets its own goja runtime, so two script steps share no state.
- The script runs synchronously for each Part. A long-running script blocks the flow.
- Target text edits on block parts are read back. Source edits are read back only when `allow_source_mutation: true`; otherwise the source is read-only. Changes to other Part types are not persisted.

## Related

- [YAML Flow Authoring](/contribute/flow-authoring): placing the step in a flow.
- [Tool Authoring with Parameter Schemas](/contribute/tool-authoring): writing the same logic as a Go tool.
- [E-06: Execution trust](/contribute/architecture/engine/e-06-execution-trust): how kapi gates a script before it runs.
