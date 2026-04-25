import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{t as o}from"./src-B5GKo4pn.js";import{n as s}from"./schema-form-ZLxpVH7H.js";function c({schema:e,description:t}){let[n,r]=(0,l.useState)({});return(0,u.jsxs)(`div`,{style:{display:`grid`,gridTemplateColumns:`1fr 1fr`,gap:24,maxWidth:900},children:[(0,u.jsxs)(`div`,{children:[t&&(0,u.jsx)(`p`,{className:`text-sm text-muted-foreground mb-4`,children:t}),(0,u.jsx)(s,{schema:e,values:n,onChange:r})]}),(0,u.jsxs)(`div`,{style:{minWidth:0},children:[(0,u.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2`,children:a(`2WRugU`,`Schema (JSON)`)}),(0,u.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-60`,children:JSON.stringify(e,null,2)}),(0,u.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2`,children:a(`1DEUwR`,`Values`)}),(0,u.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40`,children:JSON.stringify(n,null,2)})]})]})}var l,u,d,f,p,m,h,g,_,v,y;t((()=>{i(),l=e(n(),1),o(),u=r(),d={title:`Formats & Tools/Schema/Property Types`,component:c,tags:[`autodocs`],parameters:{layout:`padded`}},f={name:`string — Text Input`,args:{description:"A simple string property renders as a text input. The `description` becomes helper text below the field.",schema:{title:`String Properties`,type:`object`,properties:{name:{type:`string`,title:`Name`,description:`A simple text field`},pattern:{type:`string`,title:`Regex Pattern`,description:`With placeholder text`,"ui:placeholder":`e.g., ^[A-Z].*`},notes:{type:`string`,title:`Notes`,description:`Multiline when ui:widget is set to 'textarea'`,"ui:widget":`textarea`}}}}},p={name:`boolean — Checkbox`,args:{description:"Boolean properties render as checkboxes. The `default` value is shown when no value is set.",schema:{title:`Boolean Properties`,type:`object`,properties:{enabled:{type:`boolean`,title:`Enabled`,description:`A simple on/off toggle`,default:!0},extractAll:{type:`boolean`,title:`Extract All Pairs`,description:`Extract all string key-value pairs as translatable blocks`,default:!1}}}}},m={name:`number / integer — Numeric Input`,args:{description:"Number properties render as numeric inputs. `minimum` and `maximum` set bounds. `default` provides the initial value.",schema:{title:`Number Properties`,type:`object`,properties:{threshold:{type:`number`,title:`Similarity Threshold`,description:`Minimum match score (0.0-1.0)`,default:.7,minimum:0,maximum:1},maxSegments:{type:`integer`,title:`Max Segments`,description:`Maximum number of segments to process`,default:1e3,minimum:1}}}}},h={name:`options — Labeled Dropdown Select`,args:{description:"Properties with `options` render as dropdown selects with labeled values. Each option has a `value` and a `label`. Use `ui:enum-descriptions` for tooltips.",schema:{title:`Options Properties`,type:`object`,properties:{outputFormat:{type:`string`,title:`Output Format`,description:`Choose the output format`,options:[{value:`json`,label:`JSON`},{value:`yaml`,label:`YAML`},{value:`xml`,label:`XML`}],default:`json`},severity:{type:`string`,title:`Severity Level`,description:`With custom labels and descriptions`,options:[{value:`error`,label:`Error (fail build)`},{value:`warning`,label:`Warning (report only)`},{value:`info`,label:`Informational`}],default:`warning`,"ui:enum-descriptions":{error:`Stops processing and reports the issue as a build failure`,warning:`Reports the issue but continues processing`,info:`Logs the issue for information purposes only`}}}}}},g={name:`object — Nested Fields`,args:{description:`Object properties create nested field groups. For shallow nesting (depth 1), fields are shown inline. Deeper nesting uses drill-down navigation.`,schema:{title:`Object Properties`,type:`object`,properties:{parser:{type:`object`,title:`Parser Settings`,description:`Settings for the document parser`,properties:{assumeWellformed:{type:`boolean`,title:`Assume Well-formed`,description:`Skip validation of input structure`,default:!1},encoding:{type:`string`,title:`Input Encoding`,description:`Override auto-detected encoding`,default:`UTF-8`}}},output:{type:`object`,title:`Output`,description:`Output generation settings`,properties:{indent:{type:`integer`,title:`Indent Spaces`,default:2,minimum:0,maximum:8},trailingNewline:{type:`boolean`,title:`Trailing Newline`,default:!0}}}}}}},_={name:`array — List Editor`,args:{description:"Array properties render as lists with add/remove controls. The `items` schema defines the type of each element.",schema:{title:`Array Properties`,type:`object`,properties:{extensions:{type:`array`,title:`File Extensions`,description:`List of file extensions to process`,items:{type:`string`}},rules:{type:`array`,title:`Extraction Rules`,description:`Regex patterns for content extraction`,items:{type:`string`}}}}}},v={name:`Duplicate Label Suppression`,args:{description:"When `title` and `description` are the same (case-insensitive), the description is suppressed to avoid visual duplication. Compare the first two fields (title === description) with the third (different description).",schema:{title:`Label Suppression Demo`,type:`object`,properties:{expansionPercent:{type:`integer`,title:`Expansion Percent`,description:`Expansion percent`,default:30},prefix:{type:`string`,title:`Prefix`,description:`Prefix`,default:`[`},suffix:{type:`string`,title:`Suffix`,description:`Character appended after each translated segment`,default:`]`}}}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  name: "string — Text Input",
  args: {
    description: "A simple string property renders as a text input. The \`description\` becomes helper text below the field.",
    schema: {
      title: "String Properties",
      type: "object",
      properties: {
        name: {
          type: "string",
          title: "Name",
          description: "A simple text field"
        },
        pattern: {
          type: "string",
          title: "Regex Pattern",
          description: "With placeholder text",
          "ui:placeholder": "e.g., ^[A-Z].*"
        },
        notes: {
          type: "string",
          title: "Notes",
          description: "Multiline when ui:widget is set to 'textarea'",
          "ui:widget": "textarea"
        }
      }
    }
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  name: "boolean — Checkbox",
  args: {
    description: "Boolean properties render as checkboxes. The \`default\` value is shown when no value is set.",
    schema: {
      title: "Boolean Properties",
      type: "object",
      properties: {
        enabled: {
          type: "boolean",
          title: "Enabled",
          description: "A simple on/off toggle",
          default: true
        },
        extractAll: {
          type: "boolean",
          title: "Extract All Pairs",
          description: "Extract all string key-value pairs as translatable blocks",
          default: false
        }
      }
    }
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "number / integer — Numeric Input",
  args: {
    description: "Number properties render as numeric inputs. \`minimum\` and \`maximum\` set bounds. \`default\` provides the initial value.",
    schema: {
      title: "Number Properties",
      type: "object",
      properties: {
        threshold: {
          type: "number",
          title: "Similarity Threshold",
          description: "Minimum match score (0.0-1.0)",
          default: 0.7,
          minimum: 0,
          maximum: 1
        },
        maxSegments: {
          type: "integer",
          title: "Max Segments",
          description: "Maximum number of segments to process",
          default: 1000,
          minimum: 1
        }
      }
    }
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "options — Labeled Dropdown Select",
  args: {
    description: "Properties with \`options\` render as dropdown selects with labeled values. Each option has a \`value\` and a \`label\`. Use \`ui:enum-descriptions\` for tooltips.",
    schema: {
      title: "Options Properties",
      type: "object",
      properties: {
        outputFormat: {
          type: "string",
          title: "Output Format",
          description: "Choose the output format",
          options: [{
            value: "json",
            label: "JSON"
          }, {
            value: "yaml",
            label: "YAML"
          }, {
            value: "xml",
            label: "XML"
          }],
          default: "json"
        },
        severity: {
          type: "string",
          title: "Severity Level",
          description: "With custom labels and descriptions",
          options: [{
            value: "error",
            label: "Error (fail build)"
          }, {
            value: "warning",
            label: "Warning (report only)"
          }, {
            value: "info",
            label: "Informational"
          }],
          default: "warning",
          "ui:enum-descriptions": {
            error: "Stops processing and reports the issue as a build failure",
            warning: "Reports the issue but continues processing",
            info: "Logs the issue for information purposes only"
          }
        }
      }
    }
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "object — Nested Fields",
  args: {
    description: "Object properties create nested field groups. For shallow nesting (depth 1), fields are shown inline. Deeper nesting uses drill-down navigation.",
    schema: {
      title: "Object Properties",
      type: "object",
      properties: {
        parser: {
          type: "object",
          title: "Parser Settings",
          description: "Settings for the document parser",
          properties: {
            assumeWellformed: {
              type: "boolean",
              title: "Assume Well-formed",
              description: "Skip validation of input structure",
              default: false
            },
            encoding: {
              type: "string",
              title: "Input Encoding",
              description: "Override auto-detected encoding",
              default: "UTF-8"
            }
          }
        },
        output: {
          type: "object",
          title: "Output",
          description: "Output generation settings",
          properties: {
            indent: {
              type: "integer",
              title: "Indent Spaces",
              default: 2,
              minimum: 0,
              maximum: 8
            },
            trailingNewline: {
              type: "boolean",
              title: "Trailing Newline",
              default: true
            }
          }
        }
      }
    }
  }
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "array — List Editor",
  args: {
    description: "Array properties render as lists with add/remove controls. The \`items\` schema defines the type of each element.",
    schema: {
      title: "Array Properties",
      type: "object",
      properties: {
        extensions: {
          type: "array",
          title: "File Extensions",
          description: "List of file extensions to process",
          items: {
            type: "string"
          }
        },
        rules: {
          type: "array",
          title: "Extraction Rules",
          description: "Regex patterns for content extraction",
          items: {
            type: "string"
          }
        }
      }
    }
  }
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  name: "Duplicate Label Suppression",
  args: {
    description: "When \`title\` and \`description\` are the same (case-insensitive), the description is suppressed to avoid visual duplication. Compare the first two fields (title === description) with the third (different description).",
    schema: {
      title: "Label Suppression Demo",
      type: "object",
      properties: {
        expansionPercent: {
          type: "integer",
          title: "Expansion Percent",
          description: "Expansion percent",
          default: 30
        },
        prefix: {
          type: "string",
          title: "Prefix",
          description: "Prefix",
          default: "["
        },
        suffix: {
          type: "string",
          title: "Suffix",
          description: "Character appended after each translated segment",
          default: "]"
        }
      }
    }
  }
}`,...v.parameters?.docs?.source}}},y=[`StringProperty`,`BooleanProperty`,`NumberProperty`,`EnumProperty`,`ObjectProperty`,`ArrayProperty`,`DuplicateLabelSuppression`]}))();export{_ as ArrayProperty,p as BooleanProperty,v as DuplicateLabelSuppression,h as EnumProperty,m as NumberProperty,g as ObjectProperty,f as StringProperty,y as __namedExportsOrder,d as default};