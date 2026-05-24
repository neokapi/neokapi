import { useMemo } from "react";
import Layout from "@theme/Layout";
import { commands as commandData } from "@neokapi/reference-data";
import { commandName } from "@site/src/components/commands/commandHelpers";
import CommandGrid from "@site/src/components/commands/CommandGrid";

export default function Commands() {
  // Stable display-name order within each group; the grid handles grouping by
  // the cobra group ID.
  const commands = useMemo(
    () => [...commandData.commands].sort((a, b) => commandName(a).localeCompare(commandName(b))),
    [],
  );

  return (
    <Layout
      title="Command Reference"
      description="Interactive reference for every kapi CLI command — synopsis, flags, and examples generated from the binary, with a live Run button for commands that work offline."
    >
      <main className="container margin-vert--lg">
        <h1>Command Reference</h1>
        <p>
          Every command in the kapi CLI, generated from the binary so the flags and synopses match
          the version you have installed. Select a command to read its description, flags, and
          examples. Commands that work entirely offline carry a live Run button that executes them
          in your browser against a small sample file; commands that need network access, a saved
          credential, or a running Bowrain server point to a walkthrough instead. Each command has a
          shareable link.
        </p>

        <CommandGrid commands={commands} />
      </main>
    </Layout>
  );
}
