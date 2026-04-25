import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{t as o}from"./src-B5GKo4pn.js";import{n as s}from"./schema-form-ZLxpVH7H.js";function c({schema:e,description:t}){let[n,r]=(0,l.useState)({});return(0,u.jsxs)(`div`,{style:{display:`grid`,gridTemplateColumns:`1fr 1fr`,gap:24,maxWidth:900},children:[(0,u.jsxs)(`div`,{children:[t&&(0,u.jsx)(`p`,{className:`text-sm text-muted-foreground mb-4`,children:t}),(0,u.jsx)(s,{schema:e,values:n,onChange:r})]}),(0,u.jsxs)(`div`,{style:{minWidth:0},children:[(0,u.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2`,children:a(`1DEUwR`,`Values`)}),(0,u.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40`,children:JSON.stringify(n,null,2)}),(0,u.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2`,children:a(`3TBNvC`,`Schema`)}),(0,u.jsx)(`pre`,{className:`rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-60`,children:JSON.stringify(e,null,2)})]})]})}var l,u,d,f,p,m,h,g,_,v,y,b;t((()=>{i(),l=e(n(),1),o(),u=r(),d={title:`Formats & Tools/Schema/Widgets & Editors`,component:c,tags:[`autodocs`],parameters:{layout:`padded`}},f={name:`ui:widget: text — Standard Text Input`,args:{description:"The default for string properties. Supports `ui:placeholder` for hint text.",schema:{title:`Text Widgets`,type:`object`,properties:{name:{type:`string`,title:`Name`,"ui:widget":`text`},description:{type:`string`,title:`Description`,"ui:widget":`text`,"ui:placeholder":`Enter a description...`}}}}},p={name:`ui:widget: textarea — Multiline Text`,args:{description:'Renders a multiline text area. The canonical widget name is `"multilineText"`. The alias `"textarea"` is also accepted. Use for rules, patterns, or any long-form text input.',schema:{title:`Textarea Widget`,type:`object`,properties:{rules:{type:`string`,title:`Extraction Rules (multilineText)`,description:`Using ui:widget: multilineText`,"ui:widget":`multilineText`},notes:{type:`string`,title:`Notes (textarea alias)`,description:`Using ui:widget: textarea — normalized to multilineText`,"ui:widget":`textarea`}}}}},m={name:`ui:widget: regexBuilder — Regex Pattern Input`,args:{description:`A text input styled for regex patterns. Provides visual feedback for regex syntax.`,schema:{title:`Regex Builder`,type:`object`,properties:{pattern:{type:`string`,title:`Extraction Pattern`,description:`Regex for matching translatable content`,"ui:widget":`regexBuilder`}}}}},h={name:`ui:widget: tagList — Tag/Token Input`,args:{description:`A tag list editor for entering multiple values as visual tags.`,schema:{title:`Tag List`,type:`object`,properties:{inlineTags:{type:`string`,title:`Inline Element Names`,description:`HTML elements to treat as inline (e.g., b, i, span)`,"ui:widget":`tagList`}}}}},g={name:`ui:widget: text (password) — Masked Input`,args:{description:"The ui:widget-options metadata can specify `text.password: true` for sensitive fields.",schema:{title:`Password Field`,type:`object`,properties:{apiKey:{type:`string`,title:`API Key`,description:`Your authentication token`,"ui:widget":`password`}}}}},_={name:`ui:widget: spin — Numeric Spinner`,args:{description:`Numeric input with increment/decrement controls from ui:widget-options.`,schema:{title:`Spin Widget`,type:`object`,properties:{maxWidth:{type:`integer`,title:`Max Width`,description:`Maximum line width in characters`,default:80,minimum:20,maximum:500,"ui:widget":`spin`}}}}},v={name:`ui:widget: checkList — Named Checkbox List`,args:{description:`Renders a list of named checkboxes from the ui:widget-options.entries array. Each entry has a name, title, and optional description.`,schema:{title:`Check List`,type:`object`,properties:{features:{type:`string`,title:`Enabled Features`,description:`Select which features to enable`,"ui:widget":`checkList`,"ui:widget-options":{entries:[{name:`segmentation`,title:`Segmentation`,description:`Split text into sentences`},{name:`codeFinder`,title:`Code Finder`,description:`Detect inline codes`},{name:`subfilter`,title:`Sub-filtering`,description:`Apply secondary filter to embedded content`}]}}}}}},y={name:`ui:widget: dropdown — Dropdown Select`,args:{description:`Dropdown selection from ui:widget-options. Maps to the same rendering as enum + ui:enum-labels.`,schema:{title:`Dropdown Widget`,type:`object`,properties:{lineBreak:{type:`string`,title:`Line Break Style`,enum:[`lf`,`crlf`,`platform`],default:`platform`,"ui:widget":`dropdown`,"ui:enum-labels":{lf:`Unix (LF)`,crlf:`Windows (CRLF)`,platform:`Platform Default`}}}}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  name: "ui:widget: text — Standard Text Input",
  args: {
    description: "The default for string properties. Supports \`ui:placeholder\` for hint text.",
    schema: {
      title: "Text Widgets",
      type: "object",
      properties: {
        name: {
          type: "string",
          title: "Name",
          "ui:widget": "text"
        },
        description: {
          type: "string",
          title: "Description",
          "ui:widget": "text",
          "ui:placeholder": "Enter a description..."
        }
      }
    }
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  name: "ui:widget: textarea — Multiline Text",
  args: {
    description: 'Renders a multiline text area. The canonical widget name is \`"multilineText"\`. The alias \`"textarea"\` is also accepted. Use for rules, patterns, or any long-form text input.',
    schema: {
      title: "Textarea Widget",
      type: "object",
      properties: {
        rules: {
          type: "string",
          title: "Extraction Rules (multilineText)",
          description: "Using ui:widget: multilineText",
          "ui:widget": "multilineText"
        },
        notes: {
          type: "string",
          title: "Notes (textarea alias)",
          description: "Using ui:widget: textarea — normalized to multilineText",
          "ui:widget": "textarea"
        }
      }
    }
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "ui:widget: regexBuilder — Regex Pattern Input",
  args: {
    description: "A text input styled for regex patterns. Provides visual feedback for regex syntax.",
    schema: {
      title: "Regex Builder",
      type: "object",
      properties: {
        pattern: {
          type: "string",
          title: "Extraction Pattern",
          description: "Regex for matching translatable content",
          "ui:widget": "regexBuilder"
        }
      }
    }
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "ui:widget: tagList — Tag/Token Input",
  args: {
    description: "A tag list editor for entering multiple values as visual tags.",
    schema: {
      title: "Tag List",
      type: "object",
      properties: {
        inlineTags: {
          type: "string",
          title: "Inline Element Names",
          description: "HTML elements to treat as inline (e.g., b, i, span)",
          "ui:widget": "tagList"
        }
      }
    }
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "ui:widget: text (password) — Masked Input",
  args: {
    description: "The ui:widget-options metadata can specify \`text.password: true\` for sensitive fields.",
    schema: {
      title: "Password Field",
      type: "object",
      properties: {
        apiKey: {
          type: "string",
          title: "API Key",
          description: "Your authentication token",
          "ui:widget": "password"
        }
      }
    }
  }
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "ui:widget: spin — Numeric Spinner",
  args: {
    description: "Numeric input with increment/decrement controls from ui:widget-options.",
    schema: {
      title: "Spin Widget",
      type: "object",
      properties: {
        maxWidth: {
          type: "integer",
          title: "Max Width",
          description: "Maximum line width in characters",
          default: 80,
          minimum: 20,
          maximum: 500,
          "ui:widget": "spin"
        }
      }
    }
  }
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  name: "ui:widget: checkList — Named Checkbox List",
  args: {
    description: "Renders a list of named checkboxes from the ui:widget-options.entries array. Each entry has a name, title, and optional description.",
    schema: {
      title: "Check List",
      type: "object",
      properties: {
        features: {
          type: "string",
          title: "Enabled Features",
          description: "Select which features to enable",
          "ui:widget": "checkList",
          "ui:widget-options": {
            entries: [{
              name: "segmentation",
              title: "Segmentation",
              description: "Split text into sentences"
            }, {
              name: "codeFinder",
              title: "Code Finder",
              description: "Detect inline codes"
            }, {
              name: "subfilter",
              title: "Sub-filtering",
              description: "Apply secondary filter to embedded content"
            }]
          }
        }
      }
    }
  }
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  name: "ui:widget: dropdown — Dropdown Select",
  args: {
    description: "Dropdown selection from ui:widget-options. Maps to the same rendering as enum + ui:enum-labels.",
    schema: {
      title: "Dropdown Widget",
      type: "object",
      properties: {
        lineBreak: {
          type: "string",
          title: "Line Break Style",
          enum: ["lf", "crlf", "platform"],
          default: "platform",
          "ui:widget": "dropdown",
          "ui:enum-labels": {
            lf: "Unix (LF)",
            crlf: "Windows (CRLF)",
            platform: "Platform Default"
          }
        }
      }
    }
  }
}`,...y.parameters?.docs?.source}}},b=[`TextWidget`,`TextareaWidget`,`RegexBuilderWidget`,`TagListWidget`,`PasswordWidget`,`SpinWidget`,`CheckListWidget`,`DropdownWidget`]}))();export{v as CheckListWidget,y as DropdownWidget,g as PasswordWidget,m as RegexBuilderWidget,_ as SpinWidget,h as TagListWidget,f as TextWidget,p as TextareaWidget,b as __namedExportsOrder,d as default};