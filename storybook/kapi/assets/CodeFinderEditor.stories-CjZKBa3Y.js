import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{n as o,t as s}from"./code-finder-editor-CHHmbQA7.js";function c(e){let[t,n]=(0,l.useState)(e.initial??{rules:[],sample:``});return(0,u.jsxs)(`div`,{className:`max-w-lg`,children:[(0,u.jsx)(s,{value:t,onChange:n,presets:e.presets,label:e.label,description:e.description,disabled:e.disabled}),(0,u.jsx)(`pre`,{className:`mt-4 p-2 rounded bg-muted text-xs font-mono overflow-auto`,children:JSON.stringify(t,null,2)})]})}var l,u,d,f,p,m,h,g,_;t((()=>{i(),l=e(n(),1),o(),u=r(),d={title:`Formats & Tools/Schema/CodeFinderEditor`,component:s,tags:[`autodocs`],parameters:{docs:{description:{component:`Editor for inline code detection rules. Supports numbered regex patterns, preset selection, validation feedback, and live match highlighting against sample text.`}}}},f={render:()=>(0,u.jsx)(c,{label:a(`1c44GG`,`Inline Code Rules`),description:a(`2TRc6n`,`Define regex patterns to detect inline codes in translatable text.`)})},p={render:()=>(0,u.jsx)(c,{label:a(`1c44GG`,`Inline Code Rules`),initial:{rules:[{pattern:`</?\\w[^>]*>`},{pattern:`\\{\\d+\\}`},{pattern:`%[ds]`}],sample:`Click <b>OK</b> to format {0} with %d items`}})},m={render:()=>(0,u.jsx)(c,{label:a(`1c44GG`,`Inline Code Rules`),description:a(`xw4cI`,`Choose a preset or define custom patterns.`),presets:{"HTML Tags":{rules:[{pattern:`</?\\w[^>]*>`}],sample:`<b>Bold</b> and <i>italic</i>`},"Printf Codes":{rules:[{pattern:`%[diouxXeEfgGaAcspn%]`},{pattern:`%[-+ #0]*\\d*\\.?\\d*[diouxXeEfgGaAcspn%]`}],sample:`Found %d items in %s`},"ICU Placeholders":{rules:[{pattern:`\\{[^}]+\\}`}],sample:`Hello {name}, you have {count} messages`}}})},h={render:()=>(0,u.jsx)(c,{label:a(`1c44GG`,`Inline Code Rules`),initial:{rules:[{pattern:`</?\\w[^>]*>`},{pattern:`[invalid(`},{pattern:`\\{\\d+\\}`}],sample:`Test <b>text</b> with {0}`}})},g={render:()=>(0,u.jsx)(c,{label:a(`1c44GG`,`Inline Code Rules`),disabled:!0,initial:{rules:[{pattern:`</?\\w[^>]*>`},{pattern:`\\{\\d+\\}`}],sample:`Click <b>OK</b> to format {0}`}})},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper label="Inline Code Rules" description="Define regex patterns to detect inline codes in translatable text." />
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper label="Inline Code Rules" initial={{
    rules: [{
      pattern: "</?\\\\w[^>]*>"
    }, {
      pattern: "\\\\{\\\\d+\\\\}"
    }, {
      pattern: "%[ds]"
    }],
    sample: "Click <b>OK</b> to format {0} with %d items"
  }} />
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper label="Inline Code Rules" description="Choose a preset or define custom patterns." presets={{
    "HTML Tags": {
      rules: [{
        pattern: "</?\\\\w[^>]*>"
      }],
      sample: "<b>Bold</b> and <i>italic</i>"
    },
    "Printf Codes": {
      rules: [{
        pattern: "%[diouxXeEfgGaAcspn%]"
      }, {
        pattern: "%[-+ #0]*\\\\d*\\\\.?\\\\d*[diouxXeEfgGaAcspn%]"
      }],
      sample: "Found %d items in %s"
    },
    "ICU Placeholders": {
      rules: [{
        pattern: "\\\\{[^}]+\\\\}"
      }],
      sample: "Hello {name}, you have {count} messages"
    }
  }} />
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper label="Inline Code Rules" initial={{
    rules: [{
      pattern: "</?\\\\w[^>]*>"
    }, {
      pattern: "[invalid("
    }, {
      pattern: "\\\\{\\\\d+\\\\}"
    }],
    sample: "Test <b>text</b> with {0}"
  }} />
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper label="Inline Code Rules" disabled initial={{
    rules: [{
      pattern: "</?\\\\w[^>]*>"
    }, {
      pattern: "\\\\{\\\\d+\\\\}"
    }],
    sample: "Click <b>OK</b> to format {0}"
  }} />
}`,...g.parameters?.docs?.source}}},_=[`Empty`,`WithRules`,`WithPresets`,`InvalidRegex`,`Disabled`]}))();export{g as Disabled,f as Empty,h as InvalidRegex,m as WithPresets,p as WithRules,_ as __namedExportsOrder,d as default};