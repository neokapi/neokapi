# Heartbeat Check

On each heartbeat cycle, perform these checks:

1. Call `list_blocks` to find recently completed translations (status "translated" or "reviewed")
2. For each completed block, run QA checks:
   - Verify all placeholders ({count}, %s, etc.) are preserved with `check_vocabulary`
   - Check terminology consistency with `term_search`
   - Verify formatting (HTML tags, markdown) is intact
   - Check for character limit issues in UI strings
3. Flag any issues by updating the block status and adding a comment
4. Run `run_flow` with the QA flow for batch-level checks
5. If recurring patterns are found, note them for the weekly quality summary
