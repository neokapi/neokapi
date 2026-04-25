import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{t as o}from"./src-CFqQ2E1s.js";import{n as s}from"./schema-form-ZLxpVH7H.js";import{n as c,t as l}from"./presets-BCvXUdtq.js";function u({schema:e,description:t,presetValues:n}){let[r,i]=(0,d.useState)({});return(0,f.jsxs)(`div`,{style:{display:`grid`,gridTemplateColumns:`1fr 1fr`,gap:24,maxWidth:900},children:[(0,f.jsxs)(`div`,{children:[t&&(0,f.jsx)(`p`,{className:`text-sm text-muted-foreground mb-4`,children:t}),(0,f.jsx)(s,{schema:e,values:r,onChange:i,presetValues:n})]}),(0,f.jsxs)(`div`,{style:{minWidth:0},children:[(0,f.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2`,children:a(`KiBeU`,`Current Values`)}),(0,f.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-32`,children:JSON.stringify(r,null,2)}),n&&(0,f.jsxs)(f.Fragment,{children:[(0,f.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2`,children:a(`1IwMTP`,`Active Preset Values`)}),(0,f.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-32`,children:JSON.stringify(n,null,2)})]})]})]})}var d,f,p,m,h,g,_,v;t((()=>{i(),d=e(n(),1),o(),l(),f=r(),p={title:`Formats & Tools/Schema/Presets & Defaults`,component:u,tags:[`autodocs`],parameters:{layout:`padded`}},m={name:`default — Property Defaults`,args:{description:"Each property can have a `default` value. When the user hasn't set a value, the default is shown as placeholder/initial state. The SchemaForm tracks which values differ from defaults.",schema:{title:`Properties with Defaults`,type:`object`,properties:{encoding:{type:`string`,title:`Encoding`,default:`UTF-8`},extractAll:{type:`boolean`,title:`Extract All`,default:!0},maxSegments:{type:`integer`,title:`Max Segments`,default:1e3,minimum:1},outputFormat:{type:`string`,title:`Format`,default:`json`,enum:[`json`,`yaml`,`xml`]}}}}},h={name:`presetValues — Modified Indicator Dot`,args:{description:"When `presetValues` is provided, a colored dot appears next to fields that differ from the preset. This helps users see what they've customized. Try changing field values to see the dot appear.",presetValues:{encoding:`UTF-8`,extractAll:!0,maxSegments:1e3},schema:{title:`Preset Comparison`,type:`object`,properties:{encoding:{type:`string`,title:`Encoding`,default:`UTF-8`},extractAll:{type:`boolean`,title:`Extract All`,default:!0},maxSegments:{type:`integer`,title:`Max Segments`,default:1e3,minimum:1}}}}},g=Object.entries(c).find(([,e])=>Object.keys(e).length>1),_={name:`Real Example: Bridge Format Presets`,args:{description:g?`Real presets for format "${g[0]}". Presets are extracted from Okapi filter configurations during the transform stage and stored as separate JSON files.`:`No multi-preset format found in fixtures.`,presetValues:g?Object.values(g[1])[0]:void 0,schema:{title:g?`Presets for ${g[0]}`:`No presets`,type:`object`,properties:{info:{type:`string`,title:`Available presets`,description:g?Object.keys(g[1]).join(`, `):`none`}}}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "default — Property Defaults",
  args: {
    description: "Each property can have a \`default\` value. When the user hasn't set a value, the default is shown as placeholder/initial state. The SchemaForm tracks which values differ from defaults.",
    schema: {
      title: "Properties with Defaults",
      type: "object",
      properties: {
        encoding: {
          type: "string",
          title: "Encoding",
          default: "UTF-8"
        },
        extractAll: {
          type: "boolean",
          title: "Extract All",
          default: true
        },
        maxSegments: {
          type: "integer",
          title: "Max Segments",
          default: 1000,
          minimum: 1
        },
        outputFormat: {
          type: "string",
          title: "Format",
          default: "json",
          enum: ["json", "yaml", "xml"]
        }
      }
    }
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "presetValues — Modified Indicator Dot",
  args: {
    description: "When \`presetValues\` is provided, a colored dot appears next to fields that differ from the preset. This helps users see what they've customized. Try changing field values to see the dot appear.",
    presetValues: {
      encoding: "UTF-8",
      extractAll: true,
      maxSegments: 1000
    },
    schema: {
      title: "Preset Comparison",
      type: "object",
      properties: {
        encoding: {
          type: "string",
          title: "Encoding",
          default: "UTF-8"
        },
        extractAll: {
          type: "boolean",
          title: "Extract All",
          default: true
        },
        maxSegments: {
          type: "integer",
          title: "Max Segments",
          default: 1000,
          minimum: 1
        }
      }
    }
  }
}`,...h.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "Real Example: Bridge Format Presets",
  args: {
    description: formatWithPresets ? \`Real presets for format "\${formatWithPresets[0]}". Presets are extracted from Okapi filter configurations during the transform stage and stored as separate JSON files.\` : "No multi-preset format found in fixtures.",
    presetValues: formatWithPresets ? Object.values(formatWithPresets[1])[0] as Record<string, unknown> : undefined,
    schema: {
      title: formatWithPresets ? \`Presets for \${formatWithPresets[0]}\` : "No presets",
      type: "object",
      properties: {
        info: {
          type: "string",
          title: "Available presets",
          description: formatWithPresets ? Object.keys(formatWithPresets[1]).join(", ") : "none"
        }
      }
    }
  }
}`,..._.parameters?.docs?.source}}},v=[`DefaultValues`,`PresetIndicator`,`RealPresets`]}))();export{m as DefaultValues,h as PresetIndicator,_ as RealPresets,v as __namedExportsOrder,p as default};