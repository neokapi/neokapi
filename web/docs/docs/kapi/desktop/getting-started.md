---
sidebar_position: 3
title: Your first project
sidebar_label: First project
description: A step-by-step walkthrough for creating your first localization project in Kapi Desktop — install, create a project, add content patterns, configure AI credentials, and run your first flow.
keywords: [Kapi, desktop, first project, localization project, flow, AI credentials]
---

# Your first project

This guide walks you through creating your first localization project with Kapi Desktop.

## 1. Install

```bash
brew install --cask neokapi/tap/kapi
```

The cask bundles the `kapi` CLI. See [Installation](/get-started/installation) for other platforms and options.

## 2. Launch and Create a Project

Open Kapi Desktop. You'll see the welcome page with the neokapi logo and getting started guide.

Click **New Project** to create a Kapi project. Set:

- **Name**: Your project name (e.g., "Acme App")
- **Source Language**: `en-US`
- **Target Languages**: Add your target locales (e.g., `fr-FR`, `de-DE`)

## 3. Add Content Patterns

In the **Project** view, add content entries that map to your source files:

```yaml
content:
  - path: "src/locales/en/*.json"
    format: json
    target: "src/locales/{lang}/*.json"
```

The `{lang}` placeholder is replaced with each target locale when generating output files.

## 4. Set Up AI Credentials

Go to **Credentials** and click **Add Provider**:

1. Enter a name (e.g., "My Anthropic Key")
2. Select the provider type (Anthropic, OpenAI, or Ollama)
3. Paste your API key — it's stored securely in your OS keychain, never in the project file

## 5. Build a Flow

Go to **Flows** and click **+** to create a new flow:

```yaml
translate-and-qa:
  steps:
    - tool: ai-translate
      config:
        provider: anthropic
    - tool: qa-check
```

This creates a two-step pipeline: AI translation followed by a quality check.

## 6. Run Your Flow

Select your flow, choose input files, set the target language, and click **Run**. You'll see:

- Real-time progress per file
- Tool execution status
- Timing and block counts
- Output file locations

## 7. Save and Share

Save your Kapi project (e.g., `translation.kapi`). You can:

- Commit it to git for team sharing
- Run it from the CLI: `kapi run translate-and-qa -p translation.kapi`
- Reopen it anytime in Kapi Desktop

## 8. Install Plugins (Optional)

Go to **Plugins** to browse the plugin registry. Install the **Okapi Bridge** plugin for plugging into Okapi's filters and steps:

1. Search for "okapi"
2. Click **Install**
3. The plugin loads automatically — new formats appear in the format list

## What's Next

- [Kapi Project Files](/kapi/projects) — Full format reference
- [Kapi CLI Commands](/commands?id=run) — Run flows from the command line
- [Tool Authoring](/contribute/tool-authoring) — Create custom tools
