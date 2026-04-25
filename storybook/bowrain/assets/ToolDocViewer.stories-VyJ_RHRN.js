import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{t as i}from"./utils-BUNg-TRL.js";import{t as a}from"./src-Bvgh6dl3.js";import{Ct as o,t as s}from"./lucide-react-CMMmtkYs.js";import{t as c}from"./button-MBstDm8k.js";function l({content:e,wikiUrl:t,title:n,className:r}){let a=f.useMemo(()=>u(e),[e]);return(0,p.jsxs)(`div`,{className:i(`flex flex-col gap-4`,r),children:[(n||t)&&(0,p.jsxs)(`div`,{className:`flex items-center justify-between`,children:[n&&(0,p.jsx)(`h2`,{className:`text-lg font-semibold`,children:n}),t&&(0,p.jsx)(c,{variant:`ghost`,size:`sm`,asChild:!0,children:(0,p.jsxs)(`a`,{href:t,target:`_blank`,rel:`noopener noreferrer`,className:`gap-1.5 text-xs text-muted-foreground`,children:[`Okapi Wiki`,(0,p.jsx)(o,{className:`size-3`})]})})]}),(0,p.jsx)(`div`,{className:`prose prose-sm dark:prose-invert max-w-none`,dangerouslySetInnerHTML:{__html:a}})]})}function u(e){return e.replace(/```(\w*)\n([\s\S]*?)```/g,(e,t,n)=>`<pre><code class="language-${t}">${d(n.trim())}</code></pre>`).replace(/`([^`]+)`/g,`<code>$1</code>`).replace(/^#### (.+)$/gm,`<h4>$1</h4>`).replace(/^### (.+)$/gm,`<h3>$1</h3>`).replace(/^## (.+)$/gm,`<h2>$1</h2>`).replace(/^# (.+)$/gm,`<h1>$1</h1>`).replace(/\*\*(.+?)\*\*/g,`<strong>$1</strong>`).replace(/^> (.+)$/gm,`<blockquote>$1</blockquote>`).replace(/^- (.+)$/gm,`<li>$1</li>`).replace(/(<li>.*<\/li>\n?)+/g,e=>`<ul>${e}</ul>`).replace(/^---$/gm,`<hr>`).replace(/\n\n/g,`</p><p>`).replace(/^(.+)$/gm,e=>e.startsWith(`<`)?e:`<p>${e}</p>`)}function d(e){return e.replace(/&/g,`&amp;`).replace(/</g,`&lt;`).replace(/>/g,`&gt;`)}var f,p,m=t((()=>{a(),f=e(n(),1),s(),p=r(),l.__docgenInfo={description:`Renders full-page documentation for a tool or format.
Content is fullDoc markdown from the docs extraction pipeline.

Uses simple HTML rendering of trusted markdown content.
The content comes from our own docs pipeline (not user input),
so it is safe to render directly. For untrusted content,
use a sanitizer like DOMPurify.`,methods:[],displayName:`ToolDocViewer`,props:{content:{required:!0,tsType:{name:`string`},description:`Markdown content from fullDoc.`},wikiUrl:{required:!1,tsType:{name:`string`},description:`Wiki URL for external link.`},title:{required:!1,tsType:{name:`string`},description:`Tool/format display name for the header.`},className:{required:!1,tsType:{name:`string`},description:`Additional class name.`}}}})),h,g,_,v,y,b,x;t((()=>{m(),h=r(),g={title:`Workspace/Flow/ToolDocViewer`,component:l,tags:[`autodocs`],decorators:[e=>(0,h.jsx)(`div`,{style:{maxWidth:640,padding:16},children:(0,h.jsx)(e,{})})]},_=`# Quality Check

Configurable quality assurance checks for bilingual content: whitespace, inline codes, patterns, length, characters, terminology, and LanguageTool grammar verification.

## Parameters

### Whitespace

#### Check Leading Whitespace
Flag text units where the leading whitespace characters differ between source and target.

#### Check Trailing Whitespace
Flag text units where the trailing whitespace characters differ between source and target.

### Length

#### Check Maximum Character Length
Flag target text longer than a given percentage of source text character length.

- **Long text threshold**: Character count above which text is considered "long"
- **Percentage for long text**: Maximum allowed percentage for long text
- **Percentage for short text**: Maximum allowed percentage for short text

### Patterns

#### Source/Target Pattern Rules
Regex pattern pairs for verifying source-target consistency. Each pattern defines:
- A **source regex** to match in the source text
- A **target regex** (or \`<same>\` to reuse the source match)
- **Severity** level (Low, Medium, High)
- **Direction**: source→target or target→source

\`\`\`yaml
patterns:
  - source: "[\\\\(\\\\uFF08]"
    target: "[\\\\(\\\\uFF08]"
    severity: low
    description: "Opening parenthesis"
\`\`\`

## Limitations

- LanguageTool integration requires a running server instance
- Pattern checking may increase processing time with many patterns

## Notes

- The quality check step uses the same configuration as the CheckMate application
- Session files (.qcs) can be used to persist check results across runs
`,v={args:{content:_,title:`Quality Check`,wikiUrl:`https://okapiframework.org/wiki/index.php/Quality_Check_Step`}},y={args:{content:_}},b={args:{content:`# Segmentation

Apply SRX segmentation rules to split text units into sentences.

## Parameters

#### Segment Source Text
Segment the source text of text units using SRX rules.

#### Segment Target Text
Segment existing target text using SRX rules for the processed locales.
`,title:`Segmentation`,wikiUrl:`https://okapiframework.org/wiki/index.php/Segmentation_Step`}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  args: {
    content: sampleDoc,
    title: "Quality Check",
    wikiUrl: "https://okapiframework.org/wiki/index.php/Quality_Check_Step"
  }
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  args: {
    content: sampleDoc
  }
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  args: {
    content: \`# Segmentation

Apply SRX segmentation rules to split text units into sentences.

## Parameters

#### Segment Source Text
Segment the source text of text units using SRX rules.

#### Segment Target Text
Segment existing target text using SRX rules for the processed locales.
\`,
    title: "Segmentation",
    wikiUrl: "https://okapiframework.org/wiki/index.php/Segmentation_Step"
  }
}`,...b.parameters?.docs?.source}}},x=[`Default`,`WithoutTitle`,`ShortDoc`]}))();export{v as Default,b as ShortDoc,y as WithoutTitle,x as __namedExportsOrder,g as default};