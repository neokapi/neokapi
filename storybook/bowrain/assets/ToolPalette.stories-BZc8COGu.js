import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{i as n,n as r,r as i,t as a}from"./tools-metadata-BRce1ZED.js";var o,s,c,l,u,d,f,p,m;e((()=>{n(),a(),o=t(),{fn:s}=__STORYBOOK_MODULE_TEST__,c=r,l={title:`Flow Editor/ToolPalette`,component:i,tags:[`autodocs`],args:{onAddTool:s()},parameters:{layout:`fullscreen`},decorators:[e=>(0,o.jsx)(`div`,{style:{height:600,display:`flex`},children:(0,o.jsx)(e,{})})]},u={args:{tools:c}},d={args:{tools:c.filter(e=>!e.name.startsWith(`okapi:`))}},f={args:{tools:c.filter(e=>e.name.startsWith(`okapi:`))}},p={args:{tools:c.slice(0,8)}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    tools
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    tools: tools.filter(t => !t.name.startsWith("okapi:"))
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    tools: tools.filter(t => t.name.startsWith("okapi:"))
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    tools: tools.slice(0, 8)
  }
}`,...p.parameters?.docs?.source}}},m=[`AllTools`,`BuiltInOnly`,`OkapiOnly`,`FewTools`]}))();export{u as AllTools,d as BuiltInOnly,p as FewTools,f as OkapiOnly,m as __namedExportsOrder,l as default};