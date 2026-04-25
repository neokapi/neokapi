import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{J as n,V as r,t as i}from"./dist-M2wk2DTe.js";import{n as a,t as o}from"./thread-CSSpe8xZ.js";function s({messages:e}){return(0,c.jsx)(r,{runtime:n({messages:e,isRunning:!1,convertMessage:e=>e,onNew:async()=>{},onCancel:async()=>{}}),children:(0,c.jsx)(`div`,{className:`h-[600px] w-[500px] border rounded-lg overflow-hidden flex flex-col bg-background text-foreground`,children:(0,c.jsx)(o,{})})})}var c,l,u,d,f,p,m,h,g,_,v,y,b;e((()=>{i(),a(),c=t(),l={role:`user`,id:`md-user`,content:`Show me what the markdown rendering looks like.`,createdAt:new Date(Date.now()-6e4)},u={role:`assistant`,id:`md-headings`,content:[{type:`text`,text:`# Heading 1
## Heading 2
### Heading 3
#### Heading 4

Regular paragraph text with **bold**, *italic*, and \`inline code\`.`}],status:{type:`complete`,reason:`stop`}},d={role:`assistant`,id:`md-code`,content:[{type:`text`,text:`Here is a code example:

\`\`\`typescript
interface TranslationEntry {
  key: string;
  source: string;
  target?: string;
  status: "translated" | "untranslated" | "fuzzy";
}

function getUntranslated(entries: TranslationEntry[]): TranslationEntry[] {
  return entries.filter(e => e.status === "untranslated");
}
\`\`\`

And some inline code: \`getUntranslated(entries)\``}],status:{type:`complete`,reason:`stop`}},f={role:`assistant`,id:`md-table`,content:[{type:`text`,text:`## Translation Progress

| Language | Keys | Translated | Coverage |
|----------|------|-----------|----------|
| English  | 142  | 142       | 100%     |
| French   | 142  | 98        | 69%      |
| German   | 142  | 45        | 32%      |
| Japanese | 142  | 12        | 8%       |

> Note: Coverage below 50% is flagged for review.`}],status:{type:`complete`,reason:`stop`}},p={role:`assistant`,id:`md-lists`,content:[{type:`text`,text:`### Unordered list
- First item
- Second item with **bold**
- Third item with \`code\`

### Ordered list
1. Parse source files
2. Extract translatable strings
3. Send to translation provider
4. Merge results back

---

Here is a [link to docs](https://example.com) and a blockquote:

> Localization is not just translation. It requires cultural adaptation.`}],status:{type:`complete`,reason:`stop`}},m={title:`Bravo/Assistant UI/Markdown`,tags:[`autodocs`],parameters:{layout:`centered`}},h={render:()=>(0,c.jsx)(s,{messages:[l,u]})},g={render:()=>(0,c.jsx)(s,{messages:[l,d]})},_={render:()=>(0,c.jsx)(s,{messages:[l,f]})},v={render:()=>(0,c.jsx)(s,{messages:[l,p]})},y={render:()=>(0,c.jsx)(s,{messages:[l,u,d,f,p]})},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  render: () => <ThreadWithMarkdown messages={[userMsg, headingsMsg]} />
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  render: () => <ThreadWithMarkdown messages={[userMsg, codeBlockMsg]} />
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  render: () => <ThreadWithMarkdown messages={[userMsg, tableMsg]} />
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  render: () => <ThreadWithMarkdown messages={[userMsg, listsMsg]} />
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  render: () => <ThreadWithMarkdown messages={[userMsg, headingsMsg, codeBlockMsg, tableMsg, listsMsg]} />
}`,...y.parameters?.docs?.source}}},b=[`Headings`,`CodeBlocks`,`Tables`,`Lists`,`AllElements`]}))();export{y as AllElements,g as CodeBlocks,h as Headings,v as Lists,_ as Tables,b as __namedExportsOrder,m as default};