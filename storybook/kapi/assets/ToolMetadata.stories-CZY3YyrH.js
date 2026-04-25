import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{t as o}from"./src-B5GKo4pn.js";import{n as s}from"./schema-form-ZLxpVH7H.js";import{n as c,t as l}from"./tool-schemas-B0hhFdR6.js";function u({schema:e,description:t}){let[n,r]=(0,d.useState)({}),i=e.toolMeta;return(0,f.jsxs)(`div`,{style:{display:`grid`,gridTemplateColumns:`1fr 1fr`,gap:24,maxWidth:900},children:[(0,f.jsxs)(`div`,{children:[t&&(0,f.jsx)(`p`,{className:`text-sm text-muted-foreground mb-4`,children:t}),i&&(0,f.jsxs)(`div`,{className:`rounded-lg border p-4 mb-4 space-y-2`,children:[(0,f.jsxs)(`div`,{className:`flex items-center gap-2`,children:[(0,f.jsx)(`span`,{className:`text-sm font-semibold`,children:i.displayName||e.title}),typeof i.category==`string`?(0,f.jsx)(`span`,{className:`rounded-full bg-primary/10 text-primary px-2 py-0.5 text-xs`,children:i.category}):null]}),typeof i.description==`string`?(0,f.jsx)(`p`,{className:`text-sm text-muted-foreground`,children:i.description}):null,(0,f.jsxs)(`div`,{className:`flex flex-wrap gap-2 text-xs`,children:[i.inputs?.map(e=>(0,f.jsx)(`span`,{className:`rounded bg-muted px-1.5 py-0.5`,children:a(`2mp9ae`,`in: ${e}`,{i:e})},e)),i.outputs?.map(e=>(0,f.jsx)(`span`,{className:`rounded bg-muted px-1.5 py-0.5`,children:a(`216eeo`,`out: ${e}`,{o:e})},e)),i.tags?.map(e=>(0,f.jsx)(`span`,{className:`rounded bg-muted px-1.5 py-0.5`,children:a(`3ebyMv`,`#${e}`,{t:e})},e)),i.requires?.map(e=>(0,f.jsx)(`span`,{className:`rounded bg-destructive/10 text-destructive px-1.5 py-0.5`,children:a(`1wlo8F`,`requires: ${e}`,{r:e})},e))]})]}),(0,f.jsx)(s,{schema:e,values:n,onChange:r})]}),(0,f.jsxs)(`div`,{style:{minWidth:0},children:[(0,f.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2`,children:a(`1N1Sit`,`Tool Metadata`)}),(0,f.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40`,children:JSON.stringify(i||{},null,2)}),(0,f.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2`,children:a(`3TBNvC`,`Schema`)}),(0,f.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-60`,children:JSON.stringify(e,null,2)})]})]})}var d,f,p,m,h,g,_,v;t((()=>{i(),d=e(n(),1),o(),l(),f=r(),p={title:`Formats & Tools/Schema/Tool Metadata`,component:u,tags:[`autodocs`],parameters:{layout:`padded`}},m={name:`toolMeta — ID, Category, I/O, Tags`,args:{description:"The `toolMeta` block identifies a tool: its display name, category (translate, validate, transform, etc.), input/output part types, tags, and required context (e.g., needs target-language).",schema:{title:`Word Count`,type:`object`,toolMeta:{id:`word-count`,displayName:`Word Count`,description:`Count translatable words and segments for cost estimation`,category:`analysis`,inputs:[`block`],outputs:[`block`],tags:[`analysis`,`statistics`],requires:[`source-language`]},properties:{countWhitespace:{type:`boolean`,title:`Count Whitespace`,default:!1},includeProtected:{type:`boolean`,title:`Include Protected Content`,default:!1}}}}},h={name:`Real Example: Built-in pseudo-translate`,args:{description:`A real built-in tool schema from the neokapi Go codebase.`,schema:c.builtIn.find(e=>e[`x-name`]===`pseudo-translate`)??{title:`pseudo-translate (not found)`,type:`object`,properties:{}}}},g={name:`Real Example: Okapi Bridge search-and-replace`,args:{description:`A real Okapi bridge tool schema with toolMeta derived from step-metadata.json.`,schema:c.bridge.find(e=>e[`x-name`]===`search-and-replace`)??{title:`search-and-replace (not found)`,type:`object`,properties:{}}}},_={name:`Categories — How Tools Are Classified`,args:{description:`Tools are classified by category: translate, validate, transform, enrich, convert, pipeline, analysis. The ToolBrowser groups tools by category with distinct colors and icons.`,schema:{title:`Category Examples`,description:`The toolMeta.category field determines grouping and visual treatment in the tool browser.`,type:`object`,properties:{}}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "toolMeta — ID, Category, I/O, Tags",
  args: {
    description: "The \`toolMeta\` block identifies a tool: its display name, category (translate, validate, transform, etc.), input/output part types, tags, and required context (e.g., needs target-language).",
    schema: {
      title: "Word Count",
      type: "object",
      toolMeta: {
        id: "word-count",
        displayName: "Word Count",
        description: "Count translatable words and segments for cost estimation",
        category: "analysis",
        inputs: ["block"],
        outputs: ["block"],
        tags: ["analysis", "statistics"],
        requires: ["source-language"]
      },
      properties: {
        countWhitespace: {
          type: "boolean",
          title: "Count Whitespace",
          default: false
        },
        includeProtected: {
          type: "boolean",
          title: "Include Protected Content",
          default: false
        }
      }
    } as unknown as ComponentSchema
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "Real Example: Built-in pseudo-translate",
  args: {
    description: "A real built-in tool schema from the neokapi Go codebase.",
    schema: (toolSchemas.builtIn.find((t: Record<string, unknown>) => t["x-name"] === "pseudo-translate") ?? {
      title: "pseudo-translate (not found)",
      type: "object",
      properties: {}
    }) as unknown as ComponentSchema
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "Real Example: Okapi Bridge search-and-replace",
  args: {
    description: "A real Okapi bridge tool schema with toolMeta derived from step-metadata.json.",
    schema: (toolSchemas.bridge.find((t: Record<string, unknown>) => t["x-name"] === "search-and-replace") ?? {
      title: "search-and-replace (not found)",
      type: "object",
      properties: {}
    }) as unknown as ComponentSchema
  }
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "Categories — How Tools Are Classified",
  args: {
    description: "Tools are classified by category: translate, validate, transform, enrich, convert, pipeline, analysis. The ToolBrowser groups tools by category with distinct colors and icons.",
    schema: {
      title: "Category Examples",
      description: "The toolMeta.category field determines grouping and visual treatment in the tool browser.",
      type: "object",
      properties: {}
    }
  }
}`,..._.parameters?.docs?.source}}},v=[`ToolIdentification`,`RealBuiltInTool`,`RealBridgeTool`,`ToolCategories`]}))();export{g as RealBridgeTool,h as RealBuiltInTool,_ as ToolCategories,m as ToolIdentification,v as __namedExportsOrder,p as default};