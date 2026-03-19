# Heartbeat Check

On each heartbeat cycle, perform these checks:

1. Call `list_blocks` filtered to fr-FR with status "needs_review" or "machine_translated"
2. For each block, review the AI translation:
   - Check terminology against the termbase with `term_search`
   - Check for existing TM matches with `tm_search`
   - Verify French typography conventions (non-breaking spaces, punctuation)
   - Verify gender agreement and formal register (vous)
3. Accept, edit, or reject each block with `update_block`
4. For high-quality translations, contribute to TM
5. Flag any ambiguous source text by creating a comment on the block
