import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./RecycleBinView-BZ7FPywG.js";import{h as i,s as a}from"./fixtures-nZqLq3hB.js";var o,s,c,l,u,d,f,p;e((()=>{n(),a(),o=t(),{fn:s}=__STORYBOOK_MODULE_TEST__,c={title:`Components/RecycleBinView`,component:r,tags:[`autodocs`],decorators:[e=>(0,o.jsx)(`div`,{style:{maxWidth:800,padding:24},children:(0,o.jsx)(e,{})})]},l={args:{projects:[],onRestoreProject:s(),onPermanentlyDelete:s()}},u={args:{projects:i,onRestoreProject:s(),onPermanentlyDelete:s()}},d={args:{projects:[],loading:!0,onRestoreProject:s(),onPermanentlyDelete:s()}},f={args:{projects:i,retentionDays:7,onRestoreProject:s(),onPermanentlyDelete:s()}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    projects: [],
    onRestoreProject: fn(),
    onPermanentlyDelete: fn()
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    projects: sampleArchivedProjects,
    onRestoreProject: fn(),
    onPermanentlyDelete: fn()
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    projects: [],
    loading: true,
    onRestoreProject: fn(),
    onPermanentlyDelete: fn()
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    projects: sampleArchivedProjects,
    retentionDays: 7,
    onRestoreProject: fn(),
    onPermanentlyDelete: fn()
  }
}`,...f.parameters?.docs?.source}}},p=[`Empty`,`WithProjects`,`Loading`,`CustomRetention`]}))();export{f as CustomRetention,l as Empty,d as Loading,u as WithProjects,p as __namedExportsOrder,c as default};