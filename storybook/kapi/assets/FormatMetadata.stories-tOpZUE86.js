import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{n as o,t as s}from"./FormatConfigEditor-BTXxukX5.js";import{n as c,t as l}from"./format-schemas-uuPi4ke9.js";function u({schema:e,description:t}){let[n,r]=(0,d.useState)({});return(0,f.jsxs)(`div`,{style:{display:`grid`,gridTemplateColumns:`1fr 1fr`,gap:24,maxWidth:900},children:[(0,f.jsxs)(`div`,{children:[t&&(0,f.jsx)(`p`,{className:`text-sm text-muted-foreground mb-4`,children:t}),(0,f.jsx)(s,{schema:e,values:n,onChange:r,title:e.title})]}),(0,f.jsxs)(`div`,{style:{minWidth:0},children:[(0,f.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2`,children:a(`3bNCSH`,`formatMeta`)}),(0,f.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40`,children:JSON.stringify(e.formatMeta||{},null,2)}),(0,f.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2`,children:a(`1DEUwR`,`Values`)}),(0,f.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40`,children:JSON.stringify(n,null,2)})]})]})}var d,f,p,m,h,g,_,v;t((()=>{i(),d=e(n(),1),o(),c(),f=r(),p={title:`Formats & Tools/Schema/Format Metadata`,component:u,tags:[`autodocs`],parameters:{layout:`padded`}},m={name:`formatMeta — Format ID, Extensions, MIME Types`,args:{description:"The `formatMeta` block identifies a format: its ID, supported file extensions, and MIME types. The FormatConfigEditor renders these as badges in the header.",schema:{title:`JSON Format`,description:`Configuration for the JSON file format reader/writer`,type:`object`,formatMeta:{id:`json`,extensions:[`.json`,`.jsonc`,`.json5`],mimeTypes:[`application/json`]},properties:{extractAllPairs:{type:`boolean`,title:`Extract All Pairs`,default:!0}}}}},h={name:`Real Example: Built-in JSON Schema`,args:{description:`A real built-in format schema generated from the neokapi Go codebase. Shows how the actual schema renders.`,schema:l.builtIn.find(e=>e[`x-name`]===`json`)??{title:`JSON (not found)`,type:`object`,properties:{}}}},g={name:`Real Example: Okapi Bridge HTML Schema`,args:{description:`A real Okapi bridge format schema with formatMeta, ui:widget hints, and complex nested properties.`,schema:l.bridge.find(e=>e[`x-name`]===`okf_html`)??{title:`HTML (not found)`,type:`object`,properties:{}}}},_={name:`$ref + $defs — Schema References`,args:{description:"Properties can use `$ref` to reference shared definitions in `$defs`. The SchemaForm resolves these at render time. This is common in Okapi bridge schemas where multiple properties share the same structure.",schema:{title:`Schema with $defs`,type:`object`,$defs:{ruleEntry:{type:`object`,properties:{pattern:{type:`string`,title:`Pattern`},action:{type:`string`,title:`Action`,enum:[`extract`,`skip`,`protect`]}}}},properties:{extractionRules:{title:`Extraction Rules`,$ref:`#/$defs/ruleEntry`},protectionRules:{title:`Protection Rules`,$ref:`#/$defs/ruleEntry`}}}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "formatMeta — Format ID, Extensions, MIME Types",
  args: {
    description: "The \`formatMeta\` block identifies a format: its ID, supported file extensions, and MIME types. The FormatConfigEditor renders these as badges in the header.",
    schema: {
      title: "JSON Format",
      description: "Configuration for the JSON file format reader/writer",
      type: "object",
      formatMeta: {
        id: "json",
        extensions: [".json", ".jsonc", ".json5"],
        mimeTypes: ["application/json"]
      },
      properties: {
        extractAllPairs: {
          type: "boolean",
          title: "Extract All Pairs",
          default: true
        }
      }
    }
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "Real Example: Built-in JSON Schema",
  args: {
    description: "A real built-in format schema generated from the neokapi Go codebase. Shows how the actual schema renders.",
    schema: (formatSchemas.builtIn.find((f: Record<string, unknown>) => f["x-name"] === "json") ?? {
      title: "JSON (not found)",
      type: "object",
      properties: {}
    }) as unknown as ComponentSchema
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "Real Example: Okapi Bridge HTML Schema",
  args: {
    description: "A real Okapi bridge format schema with formatMeta, ui:widget hints, and complex nested properties.",
    schema: (formatSchemas.bridge.find((f: Record<string, unknown>) => f["x-name"] === "okf_html") ?? {
      title: "HTML (not found)",
      type: "object",
      properties: {}
    }) as unknown as ComponentSchema
  }
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "$ref + $defs — Schema References",
  args: {
    description: "Properties can use \`$ref\` to reference shared definitions in \`$defs\`. The SchemaForm resolves these at render time. This is common in Okapi bridge schemas where multiple properties share the same structure.",
    schema: {
      title: "Schema with $defs",
      type: "object",
      $defs: {
        ruleEntry: {
          type: "object",
          properties: {
            pattern: {
              type: "string",
              title: "Pattern"
            },
            action: {
              type: "string",
              title: "Action",
              enum: ["extract", "skip", "protect"]
            }
          }
        }
      },
      properties: {
        extractionRules: {
          title: "Extraction Rules",
          $ref: "#/$defs/ruleEntry"
        },
        protectionRules: {
          title: "Protection Rules",
          $ref: "#/$defs/ruleEntry"
        }
      }
    } as unknown as ComponentSchema
  }
}`,..._.parameters?.docs?.source}}},v=[`FormatIdentification`,`RealBuiltInFormat`,`RealBridgeFormat`,`RefResolution`]}))();export{m as FormatIdentification,g as RealBridgeFormat,h as RealBuiltInFormat,_ as RefResolution,v as __namedExportsOrder,p as default};