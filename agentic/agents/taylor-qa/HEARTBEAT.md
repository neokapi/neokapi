Check for completed translation batches that need QA review.

Use `list_blocks` with status "translated" or "reviewed" to find blocks
that have been completed since the last check. Run quality checks on any
new batches found.

Also check for previously reported issues that may have been fixed:
- Use `get_block` for blocks that had QA issues in prior cycles
- Verify fixes and notify translators of confirmed resolutions

Check `email.listInbox` for translator responses to your issue reports.

If no new translations to check and no pending verifications, exit quietly.
