import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./BrandFindingsList-2w5b8iY9.js";import{i,l as a}from"./fixtures-Cf-tGaH5.js";var o,s,c,l,u,d,f;e((()=>{n(),i(),o=t(),s={title:`Brand/BrandFindingsList`,component:r,tags:[`autodocs`],decorators:[e=>(0,o.jsx)(`div`,{style:{maxWidth:640,padding:24},children:(0,o.jsx)(e,{})})]},c={args:{findings:a}},l={args:{findings:[]}},u={args:{findings:[a[2]]}},d={args:{findings:a.filter(e=>e.severity===`minor`)}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    findings: sampleFindings
  }
}`,...c.parameters?.docs?.source},description:{story:`Multiple findings with mixed severities.`,...c.parameters?.docs?.description}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    findings: []
  }
}`,...l.parameters?.docs?.source},description:{story:`No findings — fully compliant.`,...l.parameters?.docs?.description}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    findings: [sampleFindings[2]]
  }
}`,...u.parameters?.docs?.source},description:{story:`Single critical finding.`,...u.parameters?.docs?.description}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    findings: sampleFindings.filter(f => f.severity === "minor")
  }
}`,...d.parameters?.docs?.source},description:{story:`Only minor findings.`,...d.parameters?.docs?.description}}},f=[`MixedSeverities`,`NoFindings`,`SingleCritical`,`MinorOnly`]}))();export{d as MinorOnly,c as MixedSeverities,l as NoFindings,u as SingleCritical,f as __namedExportsOrder,s as default};