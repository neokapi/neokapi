# Evaluation & Quality Assurance

## Overview

The agentic testing system needs two levels of quality assessment:

1. **Translation quality** — Are the translations good? How do they compare to existing community translations?
2. **Platform quality** — Is Bowrain working correctly? Are there bugs, regressions, or UX issues?

## Translation Quality Evaluation

### Automated Quality Checks

Bowrain's built-in QA pipeline catches mechanical issues:

| Check | What It Catches | Severity |
|-------|-----------------|----------|
| Placeholder consistency | Missing/extra `{variables}`, `%s`, `{{tokens}}` | Critical |
| Whitespace/formatting | Leading/trailing spaces, line break changes | Warning |
| Terminology compliance | Terms not matching termbase entries | Medium |
| Brand compliance | Tone/style violations per brand profile | Medium |
| Character limits | Translations exceeding UI constraints | High |
| Empty translations | Blocks with no target text | Critical |
| Numeric consistency | Numbers changed or reformatted | Warning |
| Punctuation | Period/comma/quote conventions per locale | Low |

### LLM-Based Quality Assessment

Use Claude to evaluate translation quality on dimensions human QA would check:

```typescript
// Quality evaluation prompt
const qualityPrompt = `
Evaluate this translation on a 1-5 scale for each dimension:

Source (en-US): "${source}"
Translation (${targetLocale}): "${translation}"
Context: ${fileContext}
Domain: ${domain}

Dimensions:
1. **Accuracy**: Does it convey the same meaning? (1=wrong, 5=perfect)
2. **Fluency**: Does it read naturally in ${targetLanguage}? (1=awkward, 5=native)
3. **Terminology**: Are technical terms translated correctly? (1=wrong, 5=consistent)
4. **Style**: Does it match the brand voice? (1=off-brand, 5=perfect match)
5. **Completeness**: Is all information preserved? (1=missing info, 5=complete)

Return JSON: { accuracy: N, fluency: N, terminology: N, style: N, completeness: N, overall: N, issues: [...] }
`;
```

### Benchmark Against Existing Translations

For projects with existing community translations (Crowdin, Weblate), compare:

```
Evaluation Matrix:
┌──────────────────┬──────────┬──────────┬──────────┐
│ Metric           │ Existing │ Bowrain  │ Delta    │
│                  │ (Crowdin)│ (Agents) │          │
├──────────────────┼──────────┼──────────┼──────────┤
│ Accuracy (avg)   │ 4.2      │ 4.1      │ -0.1     │
│ Fluency (avg)    │ 3.8      │ 4.0      │ +0.2     │
│ Terminology      │ 3.5      │ 4.3      │ +0.8 ✓   │
│ Consistency      │ 3.2      │ 4.5      │ +1.3 ✓✓  │
│ Coverage         │ 85%      │ 100%     │ +15% ✓   │
│ Time to translate│ 2 weeks  │ 3 days   │ -11 days │
└──────────────────┴──────────┴──────────┴──────────┘

Key insight: Bowrain + agents wins on terminology consistency (enforced by
termbase) and coverage (agents don't skip files), while community
translations may win on accuracy for culturally nuanced content.
```

### Quality Trends Over Time

Track how quality evolves as TM and terminology grow:

```
Quality Score Trend (French, Docusaurus):

Score
5.0 │                                          ╭──●
    │                                    ╭────╯
4.5 │                              ╭────╯
    │                        ╭────╯
4.0 │                  ╭────╯
    │            ╭────╯
3.5 │      ╭────╯
    │╭────╯
3.0 │╯
    └──────────────────────────────────────────────
    Week 1   Week 3   Week 5   Week 7   Week 9   Week 12

Factors driving improvement:
- TM reuse eliminates inconsistency in repeated phrases
- Termbase enforces correct technical terms
- Agent "learning" (previous session context) reduces errors
- Brand profiles catch tone violations earlier
```

### Human Evaluation Samples

Periodically sample translations for human review (by the actual project maintainer, not agents):

- **Sample size:** 50 blocks per language per project per month
- **Selection:** Stratified by file type, content length, and AI acceptance rate
- **Evaluation:** 1-5 scale on accuracy, fluency, appropriateness
- **Feedback loop:** Human evaluations inform agent prompt refinement

## Platform Quality (Bug Detection)

### What Agents Naturally Surface

Because agents exercise the full platform continuously, they naturally discover:

| Category | Example Issues | Detection Method |
|----------|---------------|------------------|
| API bugs | 500 errors, wrong status codes | HTTP response monitoring |
| CLI bugs | Push/pull failures, corrupt output | Exit code + stderr analysis |
| Auth issues | Token expiry, refresh failures | 401/403 responses |
| Data integrity | Lost translations, TM corruption | Before/after comparison |
| Performance | Slow responses under load | Response time tracking |
| UX friction | Confusing workflows, missing features | Agent "confusion" (LLM uncertainty) |
| Format bugs | Incorrect parsing, round-trip failures | Output file validation |
| Concurrency | Race conditions, lost writes | Parallel agent operations |

### Structured Platform Testing

Beyond natural discovery, agents can run targeted platform tests:

```typescript
// Platform health check tasks
const platformTests = {
  // Round-trip integrity
  roundTrip: async (ctx: AgentContext) => {
    // Push a file → pull it back → compare
    const original = readFile("test-fixture.json");
    await ctx.cli.push();
    await ctx.cli.pull({ locale: "en-US" });
    const roundTripped = readFile("test-fixture.json");
    assertEqual(original, roundTripped, "Round-trip integrity failed");
  },

  // Concurrent write safety
  concurrentWrites: async (ctx: AgentContext) => {
    // Two agents edit the same file simultaneously
    const p1 = agentA.editBlock(blockId, "Translation A");
    const p2 = agentB.editBlock(blockId, "Translation B");
    await Promise.all([p1, p2]);
    // Verify one write won (last-write-wins) without corruption
    const result = await ctx.api.getBlock(blockId);
    assert(result === "Translation A" || result === "Translation B");
  },

  // Stream isolation
  streamIsolation: async (ctx: AgentContext) => {
    // Edit in stream A should not affect stream B
    await ctx.api.editInStream("stream-a", blockId, "Stream A value");
    const streamBValue = await ctx.api.getBlock(blockId, { stream: "stream-b" });
    assertNotEqual(streamBValue, "Stream A value");
  },
};
```

### Bug Reporting

When agents encounter issues, generate structured reports:

```typescript
interface BugReport {
  id: string;
  severity: "critical" | "high" | "medium" | "low";
  category: "api" | "cli" | "web" | "data" | "performance";
  agent: string;
  project: string;
  timestamp: Date;
  description: string;
  steps: string[];        // Steps to reproduce
  expected: string;
  actual: string;
  context: {
    request?: object;     // HTTP request details
    response?: object;    // HTTP response details
    state?: object;       // Agent state at time of bug
    screenshot?: string;  // Screenshot if web UI issue
  };
}
```

Bug reports are:
1. Logged to a shared volume (JSON files) or Bowrain's activity feed
2. Deduplicated against known issues
3. Optionally filed as GitHub issues (with approval)
4. Tracked for regression (does this bug recur?)

## Evaluation Cadence

### Continuous (Every Session)

- HTTP error rates
- Response times
- CLI exit codes
- Basic translation completeness

### Daily

- Aggregate quality scores per language
- TM reuse rate trends
- Agent error rates
- Cost tracking

### Weekly

- LLM quality evaluation on sample blocks
- Terminology growth report
- Brand compliance audit
- Benchmark comparison (if applicable)

### Monthly

- Human evaluation sample review
- Platform bug summary
- Quality trend analysis
- Cost efficiency report
- Recommendations for prompt/config adjustments

## Feedback Loops

### Agent Self-Improvement

Agent prompts evolve based on evaluation results:

```
Evaluation shows: French translations scoring low on "formality"
  → Update Jean-Pierre's system prompt: emphasize formal register
  → Add example of formal vs. informal translations to prompt
  → Re-evaluate after 1 week

Evaluation shows: German terminology inconsistency
  → Brand Manager agent gets new task: audit German termbase
  → Add more context to Katrin's prompt about termbase usage
  → Create QA rule specifically for term consistency
```

### Platform Improvement

Agent activity drives Bowrain platform improvements:

```
Agents frequently hit: "push fails when file has BOM"
  → File bug against Bowrain → Fix reader to handle BOM
  → Agents automatically benefit on next session

Agents report: "no way to assign tasks in bulk via API"
  → Feature request → Add batch task creation endpoint
  → PM agent uses new endpoint → faster task creation
```

This creates a virtuous cycle: agents surface real issues → platform improves → agents work better → find subtler issues.
