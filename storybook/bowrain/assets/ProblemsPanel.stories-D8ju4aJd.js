import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./ProblemsPanel-YwxnV659.js";import{P as i,l as a}from"./fixtures-D8OXoGLB.js";var o,s,c,l,u,d,f,p,m,h;e((()=>{n(),a(),o=t(),{fn:s}=__STORYBOOK_MODULE_TEST__,c={title:`Editor/Terminology/ProblemsPanel`,component:r,tags:[`autodocs`],args:{onNavigateToBlock:s(),onClose:s()},parameters:{layout:`fullscreen`},decorators:[e=>(0,o.jsxs)(`div`,{style:{height:`100vh`,position:`relative`},children:[(0,o.jsx)(`div`,{style:{padding:24,color:`var(--foreground)`},children:(0,o.jsx)(`p`,{children:`Document content above the problems panel...`})}),(0,o.jsx)(e,{})]})]},l={args:{issues:i}},u={args:{issues:[]}},d={args:{issues:[],loading:!0}},f={args:{issues:[{blockId:`blk-2`,issues:[{type:`missing-tag`,severity:`error`,message:`Missing closing <b> tag in target`}]},{blockId:`blk-6`,issues:[{type:`placeholder`,severity:`error`,message:`Missing placeholder {count} in target`},{type:`punctuation`,severity:`error`,message:`Target ends with "." but source does not`}]}]}},p={args:{issues:[{blockId:`blk-1`,issues:[{type:`terminology`,severity:`warning`,message:`"localization" should be "localisation"`}]},{blockId:`blk-3`,issues:[{type:`whitespace`,severity:`warning`,message:`Trailing whitespace in target`},{type:`capitalization`,severity:`warning`,message:`Target starts with lowercase but source starts with uppercase`}]}]}},m={args:{issues:Array.from({length:10},(e,t)=>({blockId:`blk-${t+1}`,issues:[{type:`tag-mismatch`,severity:t%3==0?`error`:`warning`,message:`Issue in block ${t+1}: tag mismatch detected`},{type:`length`,severity:`warning`,message:`Block ${t+1}: target is 40% longer than source`}]}))}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    issues: sampleFileQAResults
  }
}`,...l.parameters?.docs?.source},description:{story:`Panel with mixed errors and warnings`,...l.parameters?.docs?.description}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    issues: []
  }
}`,...u.parameters?.docs?.source},description:{story:`No issues found — shows success state`,...u.parameters?.docs?.description}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    issues: [],
    loading: true
  }
}`,...d.parameters?.docs?.source},description:{story:`Loading state while QA checks are running`,...d.parameters?.docs?.description}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    issues: [{
      blockId: "blk-2",
      issues: [{
        type: "missing-tag",
        severity: "error",
        message: "Missing closing <b> tag in target"
      }]
    }, {
      blockId: "blk-6",
      issues: [{
        type: "placeholder",
        severity: "error",
        message: "Missing placeholder {count} in target"
      }, {
        type: "punctuation",
        severity: "error",
        message: 'Target ends with "." but source does not'
      }]
    }]
  }
}`,...f.parameters?.docs?.source},description:{story:`Errors only — multiple error-severity issues`,...f.parameters?.docs?.description}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    issues: [{
      blockId: "blk-1",
      issues: [{
        type: "terminology",
        severity: "warning",
        message: '"localization" should be "localisation"'
      }]
    }, {
      blockId: "blk-3",
      issues: [{
        type: "whitespace",
        severity: "warning",
        message: "Trailing whitespace in target"
      }, {
        type: "capitalization",
        severity: "warning",
        message: "Target starts with lowercase but source starts with uppercase"
      }]
    }]
  }
}`,...p.parameters?.docs?.source},description:{story:`Warnings only — no errors`,...p.parameters?.docs?.description}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    issues: Array.from({
      length: 10
    }, (_, i) => ({
      blockId: \`blk-\${i + 1}\`,
      issues: [{
        type: "tag-mismatch",
        severity: (i % 3 === 0 ? "error" : "warning") as "error" | "warning",
        message: \`Issue in block \${i + 1}: tag mismatch detected\`
      }, {
        type: "length",
        severity: "warning" as const,
        message: \`Block \${i + 1}: target is 40% longer than source\`
      }]
    }))
  }
}`,...m.parameters?.docs?.source},description:{story:`Many issues — tests scroll behavior`,...m.parameters?.docs?.description}}},h=[`WithIssues`,`NoIssues`,`Loading`,`ErrorsOnly`,`WarningsOnly`,`ManyIssues`]}))();export{f as ErrorsOnly,d as Loading,m as ManyIssues,u as NoIssues,p as WarningsOnly,l as WithIssues,h as __namedExportsOrder,c as default};