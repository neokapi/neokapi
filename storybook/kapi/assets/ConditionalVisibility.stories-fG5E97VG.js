import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{t as o}from"./src-B5GKo4pn.js";import{n as s}from"./schema-form-ZLxpVH7H.js";function c({schema:e,description:t,initialValues:n}){let[r,i]=(0,l.useState)(n??{});return(0,u.jsxs)(`div`,{style:{display:`grid`,gridTemplateColumns:`1fr 1fr`,gap:24,maxWidth:900},children:[(0,u.jsxs)(`div`,{children:[t&&(0,u.jsx)(`p`,{className:`text-sm text-muted-foreground mb-4`,children:t}),(0,u.jsx)(s,{schema:e,values:r,onChange:i})]}),(0,u.jsxs)(`div`,{style:{minWidth:0},children:[(0,u.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2`,children:a(`KiBeU`,`Current Values`)}),(0,u.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40`,children:JSON.stringify(r,null,2)}),(0,u.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2`,children:a(`3TBNvC`,`Schema`)}),(0,u.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-60`,children:JSON.stringify(e,null,2)})]})]})}var l,u,d,f,p,m,h,g,_;t((()=>{i(),l=e(n(),1),o(),u=r(),d={title:`Formats & Tools/Schema/Conditional Visibility`,component:c,tags:[`autodocs`],parameters:{layout:`padded`}},f={name:`ui:visible — Toggle Field Visibility`,args:{description:'The `ui:visible` rule on a property controls its visibility based on another field\'s value. Toggle "Use Code Finder" to show/hide the rules field.',schema:{title:`Conditional Fields`,type:`object`,properties:{useCodeFinder:{type:`boolean`,title:`Use Code Finder`,description:`Enable inline code detection`,default:!1},codeFinderRules:{type:`string`,title:`Code Finder Rules`,description:`Regex patterns for inline codes (one per line)`,"ui:widget":`textarea`,"ui:visible":{field:`useCodeFinder`,eq:!0}}}}}},p={name:`ui:visible — Enum-Driven Visibility`,args:{description:`Fields can be shown conditionally based on an enum value. Select different output modes to see different options appear.`,initialValues:{outputMode:`file`},schema:{title:`Mode-Dependent Fields`,type:`object`,properties:{outputMode:{type:`string`,title:`Output Mode`,enum:[`file`,`stdout`,`api`],default:`file`},outputPath:{type:`string`,title:`Output Path`,description:`File path for output`,"ui:visible":{field:`outputMode`,eq:`file`}},apiEndpoint:{type:`string`,title:`API Endpoint`,description:`URL to POST results to`,"ui:placeholder":`https://api.example.com/results`,"ui:visible":{field:`outputMode`,eq:`api`}},apiKey:{type:`string`,title:`API Key`,"ui:visible":{field:`outputMode`,eq:`api`}}}}}},m={name:`ui:visible empty — Show When Field is Unset`,args:{description:'Setting `empty: true` in ui:visible shows the field when the referenced field is empty or unset. Clear the "Override Path" to see the default path message.',schema:{title:`Empty-Based Visibility`,type:`object`,properties:{overridePath:{type:`string`,title:`Override Path`,description:`Custom output path (leave empty for default)`},defaultPathInfo:{type:`string`,title:`Default Path`,description:`Using auto-generated path based on input file`,"ui:visible":{field:`overridePath`,empty:!0}}}}}},h={name:`ui:enabled — Enable/Disable Fields`,args:{description:'The `ui:enabled` condition controls whether a field is interactive. The field remains visible but grayed out and non-interactive when disabled. Toggle "Enable Translation" to see the language fields enable/disable.',schema:{title:`Enabled-By Dependencies`,type:`object`,properties:{useTranslation:{type:`boolean`,title:`Enable Translation`,default:!1},sourceLanguage:{type:`string`,title:`Source Language`,default:`en`,"ui:enabled":{field:`useTranslation`,eq:!0}},targetLanguage:{type:`string`,title:`Target Language`,"ui:placeholder":`e.g., fr`,"ui:enabled":{field:`useTranslation`,eq:!0}}}}}},g={name:`ui:enabled — Nested Object (Real Filter Pattern)`,args:{description:'In real filter schemas, related settings are grouped in nested objects (often via `$defs`). The `ui:enabled` condition references sibling fields within the same nested scope. Toggle "Enable Inline Code Detection" to enable/disable the rules and merge settings.',schema:{title:`Nested Enabled-By`,type:`object`,properties:{inlineCodes:{type:`object`,title:`Inline Codes`,description:`Pattern-based detection of inline codes within translatable text.`,properties:{enabled:{type:`boolean`,title:`Enable Inline Code Detection`,description:`Enable pattern-based detection of inline codes`,default:!1},rules:{type:`string`,title:`Code Finder Rules`,description:`Regex patterns for inline codes (one per line)`,"ui:widget":`textarea`,"ui:enabled":{field:`enabled`,eq:!0}},mergeAdjacent:{type:`boolean`,title:`Merge Adjacent Codes`,description:`Merge consecutive inline codes into a single placeholder`,"ui:enabled":{field:`enabled`,eq:!0}}}}}}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  name: "ui:visible — Toggle Field Visibility",
  args: {
    description: 'The \`ui:visible\` rule on a property controls its visibility based on another field\\'s value. Toggle "Use Code Finder" to show/hide the rules field.',
    schema: {
      title: "Conditional Fields",
      type: "object",
      properties: {
        useCodeFinder: {
          type: "boolean",
          title: "Use Code Finder",
          description: "Enable inline code detection",
          default: false
        },
        codeFinderRules: {
          type: "string",
          title: "Code Finder Rules",
          description: "Regex patterns for inline codes (one per line)",
          "ui:widget": "textarea",
          "ui:visible": {
            field: "useCodeFinder",
            eq: true
          }
        }
      }
    }
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  name: "ui:visible — Enum-Driven Visibility",
  args: {
    description: "Fields can be shown conditionally based on an enum value. Select different output modes to see different options appear.",
    initialValues: {
      outputMode: "file"
    },
    schema: {
      title: "Mode-Dependent Fields",
      type: "object",
      properties: {
        outputMode: {
          type: "string",
          title: "Output Mode",
          enum: ["file", "stdout", "api"],
          default: "file"
        },
        outputPath: {
          type: "string",
          title: "Output Path",
          description: "File path for output",
          "ui:visible": {
            field: "outputMode",
            eq: "file"
          }
        },
        apiEndpoint: {
          type: "string",
          title: "API Endpoint",
          description: "URL to POST results to",
          "ui:placeholder": "https://api.example.com/results",
          "ui:visible": {
            field: "outputMode",
            eq: "api"
          }
        },
        apiKey: {
          type: "string",
          title: "API Key",
          "ui:visible": {
            field: "outputMode",
            eq: "api"
          }
        }
      }
    }
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "ui:visible empty — Show When Field is Unset",
  args: {
    description: 'Setting \`empty: true\` in ui:visible shows the field when the referenced field is empty or unset. Clear the "Override Path" to see the default path message.',
    schema: {
      title: "Empty-Based Visibility",
      type: "object",
      properties: {
        overridePath: {
          type: "string",
          title: "Override Path",
          description: "Custom output path (leave empty for default)"
        },
        defaultPathInfo: {
          type: "string",
          title: "Default Path",
          description: "Using auto-generated path based on input file",
          "ui:visible": {
            field: "overridePath",
            empty: true
          }
        }
      }
    }
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "ui:enabled — Enable/Disable Fields",
  args: {
    description: 'The \`ui:enabled\` condition controls whether a field is interactive. The field remains visible but grayed out and non-interactive when disabled. Toggle "Enable Translation" to see the language fields enable/disable.',
    schema: {
      title: "Enabled-By Dependencies",
      type: "object",
      properties: {
        useTranslation: {
          type: "boolean",
          title: "Enable Translation",
          default: false
        },
        sourceLanguage: {
          type: "string",
          title: "Source Language",
          default: "en",
          "ui:enabled": {
            field: "useTranslation",
            eq: true
          }
        },
        targetLanguage: {
          type: "string",
          title: "Target Language",
          "ui:placeholder": "e.g., fr",
          "ui:enabled": {
            field: "useTranslation",
            eq: true
          }
        }
      }
    }
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "ui:enabled — Nested Object (Real Filter Pattern)",
  args: {
    description: 'In real filter schemas, related settings are grouped in nested objects (often via \`$defs\`). The \`ui:enabled\` condition references sibling fields within the same nested scope. Toggle "Enable Inline Code Detection" to enable/disable the rules and merge settings.',
    schema: {
      title: "Nested Enabled-By",
      type: "object",
      properties: {
        inlineCodes: {
          type: "object",
          title: "Inline Codes",
          description: "Pattern-based detection of inline codes within translatable text.",
          properties: {
            enabled: {
              type: "boolean",
              title: "Enable Inline Code Detection",
              description: "Enable pattern-based detection of inline codes",
              default: false
            },
            rules: {
              type: "string",
              title: "Code Finder Rules",
              description: "Regex patterns for inline codes (one per line)",
              "ui:widget": "textarea",
              "ui:enabled": {
                field: "enabled",
                eq: true
              }
            },
            mergeAdjacent: {
              type: "boolean",
              title: "Merge Adjacent Codes",
              description: "Merge consecutive inline codes into a single placeholder",
              "ui:enabled": {
                field: "enabled",
                eq: true
              }
            }
          }
        }
      }
    }
  }
}`,...g.parameters?.docs?.source}}},_=[`ShowIfBoolean`,`ShowIfEnum`,`ShowIfEmpty`,`EnabledBy`,`EnabledByNested`]}))();export{h as EnabledBy,g as EnabledByNested,f as ShowIfBoolean,m as ShowIfEmpty,p as ShowIfEnum,_ as __namedExportsOrder,d as default};