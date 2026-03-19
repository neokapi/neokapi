# Bowrain Agentic Testing — Team Roster

This is the full team of agent personas that participate in the Bowrain agentic
testing system. Each agent runs as an independent ZeroClaw instance in its own
Docker container.

## Team Members

### 1. Alex Chen — Developer (DevOps / L10n Engineer)

- **Role:** Manages CI/CD pipelines, Bowrain CLI integration, pushes source content, pulls translations
- **Provider:** Azure OpenAI GPT-4o-mini
- **Schedule:** Weekdays 09:00 (push), 17:00 (pull)
- **Container:** `alex-developer`
- **Capabilities:** git, gh, bowrain CLI, connector tools

### 2. Maria Santos — Brand Manager (Source-Language Maintainer)

- **Role:** Owns English brand voice and terminology, curates termbase, runs brand compliance checks
- **Provider:** Azure AI Foundry Claude Sonnet
- **Schedule:** Mon/Wed/Fri 10:00 (terminology review), Thursday 10:00 (brand audit)
- **Container:** `maria-brand`
- **Languages:** en-US (source)

### 3. Jean-Pierre Dubois — French Translator

- **Role:** Translates en-US to fr-FR, reviews AI translations, builds translation memory
- **Provider:** Azure AI Foundry Claude Sonnet
- **Schedule:** Weekdays 14:00-18:00
- **Container:** `jeanpierre-fr`
- **Languages:** en-US -> fr-FR
- **Style:** Formal register (vous), precision-focused

### 4. Katrin Weber — German Translator

- **Role:** Translates en-US to de-DE, reviews AI translations, focuses on technical accuracy
- **Provider:** Azure AI Foundry Claude Sonnet
- **Schedule:** Weekdays 09:00-13:00
- **Container:** `katrin-de`
- **Languages:** en-US -> de-DE
- **Style:** Engineering background, compound-word precision

### 5. Yuki Tanaka — Japanese Translator

- **Role:** Translates en-US to ja-JP, cultural adaptation, UX-aware localization
- **Provider:** Azure AI Foundry Claude Sonnet
- **Schedule:** Weekdays 10:00-14:00 (JST)
- **Container:** `yuki-ja`
- **Languages:** en-US -> ja-JP
- **Style:** Localization specialist, space-constraint aware

### 6. Lisa Chen — Project Manager (L10n Program Manager)

- **Role:** Coordinates translation effort, creates tasks, monitors progress, manages deadlines
- **Provider:** Azure OpenAI GPT-4o
- **Schedule:** Weekdays 08:00-10:00 (dashboard review), Friday (weekly report)
- **Container:** `lisa-pm`
- **Capabilities:** Task management, progress monitoring, cross-team coordination

### 7. Taylor Kim — QA Specialist

- **Role:** Runs quality checks on translations, catches formatting errors, terminology violations
- **Provider:** Azure OpenAI GPT-4o
- **Schedule:** Heartbeat-driven every 2 hours, Monday (weekly QA report)
- **Container:** `taylor-qa`
- **Capabilities:** Placeholder verification, brand compliance, format validation

## Communication Channels

- **Bowrain Platform** (primary) — Tasks, activity feed, translation editor, brand dashboard
- **Email** (coordination) — Weekly status digests, terminology discussions, escalations
- **GitHub Issues** (feedback) — Bug reports and feature requests filed against `neokapi/agent-feedback`

## Email Directory

| Role | Name | Email Alias |
|------|------|-------------|
| Developer | Alex Chen | developer |
| Brand Manager | Maria Santos | brand-manager |
| Translator (fr-FR) | Jean-Pierre Dubois | translator-fr |
| Translator (de-DE) | Katrin Weber | translator-de |
| Translator (ja-JP) | Yuki Tanaka | translator-ja |
| Project Manager | Lisa Chen | pm |
| QA Specialist | Taylor Kim | qa |
| Everyone | — | all |
