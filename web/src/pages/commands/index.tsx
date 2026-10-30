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
      description="Interactive reference for every kapi CLI command: synopsis, flags, and examples generated from the binary, with a live Run button for commands that work in the browser."
    >
      <main className="container margin-vert--lg">
        <h1>Command Reference</h1>
        <p>
          Every command in the kapi CLI, generated from the binary so the flags and synopses match
          the version you have installed. Select a command to read its description, flags, and
          examples. Commands that run entirely in the browser carry a <strong>Run</strong> button
          that executes them against a small sample file. AI and MT commands carry a{" "}
          <strong>Demo</strong> badge and run against a built-in stub, so their output is
          illustrative rather than from a real model. Commands that need network access, an API key,
          or local system access point to a walkthrough instead. Each command has a shareable link.
        </p>
        <p>
          The groups are the ones the installed kapi prints in its help. In every project,{" "}
          <code>kapi --help</code> shows Work, led by <code>look</code>, <code>check</code>,{" "}
          <code>fix</code>, <code>rules</code>, <code>connect</code>, <code>share</code> and{" "}
          <code>open</code>, and Assets. Languages appears there when the recipe declares a target
          language, and <code>kapi help languages</code> lists it in any project. Advanced is listed
          under <code>kapi help --all</code>. Every command runs whether or not the default help
          lists it. Below, each group lists its commands alphabetically.
        </p>

        <CommandGrid commands={commands} />
      </main>
    </Layout>
  );
}
