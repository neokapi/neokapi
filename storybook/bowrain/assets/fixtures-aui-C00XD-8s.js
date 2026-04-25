import{n as e}from"./chunk-DnJy8xQt.js";var t,n,r,i,a,o,s,c=e((()=>{t={role:`user`,id:`msg-1`,content:`Can you pseudo-translate the French files?`,createdAt:new Date(Date.now()-3e5)},n={role:`assistant`,id:`msg-2`,content:[{type:`text`,text:`Sure! I'll run the pseudo-translate flow on the French target files.

Here's a quick script to verify the output:
\`\`\`python
import json
with open("fr-FR.json") as f:
    data = json.load(f)
print(f"Keys: {len(data)}")
\`\`\``},{type:`tool-call`,toolCallId:`tc-1`,toolName:`run_flow`,args:{flow:`pseudo-translate`,target_lang:`qps`},result:{blocks_processed:42,blocks_skipped:3}}],createdAt:new Date(Date.now()-24e4),status:{type:`complete`,reason:`stop`},metadata:{custom:{input_tokens:1500,output_tokens:480}}},r={role:`assistant`,id:`msg-3`,content:[{type:`text`,text:`Done! 42 blocks were pseudo-translated. I'd like to push the results to git — shall I?`},{type:`tool-call`,toolCallId:`tc-2`,toolName:`connector_push`,args:{connector_id:`git-main`,project_id:`proj-1`}}],createdAt:new Date(Date.now()-18e4),status:{type:`requires-action`,reason:`tool-calls`}},i={role:`assistant`,id:`__streaming__`,content:[{type:`text`,text:`I'm looking through your project files to find all translatable content...`}],status:{type:`running`}},a=[t,n,r],o=[t,i],s={role:`assistant`,id:`msg-md`,content:[{type:`text`,text:`## Translation Summary

Here's what I found in your project:

| File | Keys | Translated |
|------|------|-----------|
| en-US.json | 142 | — |
| fr-FR.json | 142 | 98 (69%) |
| de-DE.json | 142 | 45 (32%) |

### Key observations

1. **French** is mostly complete but has some missing keys
2. **German** needs significant work
3. All files use valid JSON with \`"key": "value"\` format

\`\`\`json
{
  "greeting": "Bonjour, {name}!",
  "farewell": "Au revoir!"
}
\`\`\`

> Note: The greeting key uses ICU message format for interpolation.`}],createdAt:new Date(Date.now()-12e4),status:{type:`complete`,reason:`stop`}}}));export{t as a,o as i,s as n,a as r,c as t};