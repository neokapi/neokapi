import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{t as i}from"./src-Bvgh6dl3.js";import{t as a}from"./code-finder-editor-CWAf_yuB.js";function o(e){let[t,n]=(0,s.useState)(e.initial??{rules:[],sample:``});return(0,c.jsxs)(`div`,{className:`max-w-lg`,children:[(0,c.jsx)(a,{value:t,onChange:n,presets:e.presets,label:e.label,description:e.description,disabled:e.disabled}),(0,c.jsx)(`pre`,{className:`mt-4 p-2 rounded bg-muted text-xs font-mono overflow-auto`,children:JSON.stringify(t,null,2)})]})}var s,c,l,u,d,f,p,m,h;t((()=>{i(),s=e(n(),1),c=r(),l={title:`UI/CodeFinderEditor`,component:a,tags:[`autodocs`],parameters:{docs:{description:{component:`Editor for inline code detection rules. Supports numbered regex patterns, preset selection, validation feedback, and live match highlighting against sample text.`}}}},u={render:()=>(0,c.jsx)(o,{label:`Inline Code Rules`,description:`Define regex patterns to detect inline codes in translatable text.`})},d={render:()=>(0,c.jsx)(o,{label:`Inline Code Rules`,initial:{rules:[{pattern:`</?\\w[^>]*>`},{pattern:`\\{\\d+\\}`},{pattern:`%[ds]`}],sample:`Click <b>OK</b> to format {0} with %d items`}})},f={render:()=>(0,c.jsx)(o,{label:`Inline Code Rules`,description:`Choose a preset or define custom patterns.`,presets:{"HTML Tags":{rules:[{pattern:`</?\\w[^>]*>`}],sample:`<b>Bold</b> and <i>italic</i>`},"Printf Codes":{rules:[{pattern:`%[diouxXeEfgGaAcspn%]`},{pattern:`%[-+ #0]*\\d*\\.?\\d*[diouxXeEfgGaAcspn%]`}],sample:`Found %d items in %s`},"ICU Placeholders":{rules:[{pattern:`\\{[^}]+\\}`}],sample:`Hello {name}, you have {count} messages`}}})},p={render:()=>(0,c.jsx)(o,{label:`Inline Code Rules`,initial:{rules:[{pattern:`</?\\w[^>]*>`},{pattern:`[invalid(`},{pattern:`\\{\\d+\\}`}],sample:`Test <b>text</b> with {0}`}})},m={render:()=>(0,c.jsx)(o,{label:`Inline Code Rules`,disabled:!0,initial:{rules:[{pattern:`</?\\w[^>]*>`},{pattern:`\\{\\d+\\}`}],sample:`Click <b>OK</b> to format {0}`}})},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper label="Inline Code Rules" description="Define regex patterns to detect inline codes in translatable text." />
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
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
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
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
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
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
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper label="Inline Code Rules" disabled initial={{
    rules: [{
      pattern: "</?\\\\w[^>]*>"
    }, {
      pattern: "\\\\{\\\\d+\\\\}"
    }],
    sample: "Click <b>OK</b> to format {0}"
  }} />
}`,...m.parameters?.docs?.source}}},h=[`Empty`,`WithRules`,`WithPresets`,`InvalidRegex`,`Disabled`]}))();export{m as Disabled,u as Empty,p as InvalidRegex,f as WithPresets,d as WithRules,h as __namedExportsOrder,l as default};