// ScriptCodeEditor — the lab's Monaco embed for the `script` tool, shared by
// ScriptLab (the standalone widget) and ScriptStepPanel (the flow workspace's
// config panel for a script step). It carries all the hard-won embed fixes:
// pinned Monaco version, TS language service fed the script API .d.ts (typed
// completions for part/emit/skip/log), DOM lib dropped (no browser globals in
// the goja sandbox), word suggestions off, EditContext off, and macOS text
// substitutions blocked at the input layer.

import React, { useEffect, useState } from "react";
import Editor, { loader } from "@monaco-editor/react";
import { SCRIPT_API_DTS } from "./scriptApi";

// Pin a specific, stable Monaco from the CDN rather than @latest. "Latest"
// (0.55.x) regressed editor behavior in this embed — EditContext eating Space,
// and document words leaking into completions the suggest options don't gate.
// 0.52.2 honors those options. Configured once at module load (this is a
// client-only, lazily-imported chunk, so it runs before the editor boots).
loader.config({ paths: { vs: "https://cdn.jsdelivr.net/npm/monaco-editor@0.52.2/min/vs" } });

function currentTheme(): "vs-dark" | "light" {
  if (typeof document === "undefined") return "light";
  return document.documentElement.getAttribute("data-theme") === "dark" ? "vs-dark" : "light";
}

export interface ScriptCodeEditorProps {
  code: string;
  onChange: (code: string) => void;
  height?: string;
}

export default function ScriptCodeEditor({
  code,
  onChange,
  height = "300px",
}: ScriptCodeEditorProps): React.ReactElement {
  const [theme, setTheme] = useState<"vs-dark" | "light">("light");

  // Follow the site's light/dark mode.
  useEffect(() => {
    setTheme(currentTheme());
    const obs = new MutationObserver(() => setTheme(currentTheme()));
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    return () => obs.disconnect();
  }, []);

  return (
    <Editor
      height={height}
      language="typescript"
      theme={theme}
      value={code}
      onChange={(v) => onChange(v ?? "")}
      onMount={(editor, monaco) => {
        // Use the TypeScript service (the model language is "typescript").
        // In JS mode, member access offers every in-scope identifier (JS is
        // dynamic), so "part." leaked words like toUpperCase; TS resolves it
        // to only the real Part members.
        const tsd = monaco.languages.typescript.typescriptDefaults;
        // The script API .d.ts gives completion, hover and inline diagnostics
        // for part / emit / skip / log.
        tsd.addExtraLib(SCRIPT_API_DTS, "file:///script-api.d.ts");
        // Drop the DOM lib so browser globals (window, document, …) — which
        // the goja sandbox does not have — stop appearing in completions.
        tsd.setCompilerOptions({ ...tsd.getCompilerOptions(), lib: ["es2020"] });
        editor.updateOptions({ wordBasedSuggestions: "off", suggest: { showWords: false } });
        // Block OS text substitutions from injecting characters into code —
        // notably macOS "Add period with double-space", which turns a double
        // space into ". ". Autocorrect-style replacements are never wanted in
        // a code editor, and the textarea's autocorrect="off" doesn't stop
        // this one, so we cancel it at the input layer.
        const ta = editor.getDomNode()?.querySelector("textarea");
        ta?.addEventListener("beforeinput", (e) => {
          const ie = e as InputEvent;
          if (
            ie.inputType === "insertReplacementText" ||
            (ie.inputType === "insertText" && ie.data === ". ")
          ) {
            e.preventDefault();
          }
        });
      }}
      options={{
        minimap: { enabled: false },
        fontSize: 13,
        lineNumbers: "on",
        scrollBeyondLastLine: false,
        automaticLayout: true,
        tabSize: 2,
        padding: { top: 10, bottom: 10 },
        // Use the classic hidden-textarea input path rather than the newer
        // EditContext API: EditContext mishandles some keys (e.g. Space)
        // when the editor sits inside certain page layouts.
        editContext: false,
        // Render the suggest/hover widgets in a body-level layer so they are
        // not clipped by the editor's fixed height + rounded overflow box.
        fixedOverflowWidgets: true,
        // Only offer type-aware completions from the script API .d.ts — not
        // every word in the document (which surfaced bogus members like
        // "toUpperCase" on `part`). showWords:false is the version-proof
        // switch; wordBasedSuggestions covers older/newer Monaco too.
        wordBasedSuggestions: "off",
        suggest: { showWords: false },
      }}
    />
  );
}
