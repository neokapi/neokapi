import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{lt as n,ut as r}from"./iframe-BJVhYAGS.js";import{i,m as a,t as o,u as s}from"./fixtures-Cf-tGaH5.js";import{n as c,r as l}from"./decorators-BptasVUO.js";var u,d,f,p,m,h,g,_;e((()=>{r(),c(),i(),u=t(),{fn:d}=__STORYBOOK_MODULE_TEST__,f={title:`Brand/BrandProfileList`,component:n,tags:[`autodocs`],decorators:[l,e=>(0,u.jsx)(`div`,{style:{maxWidth:960,padding:24},children:(0,u.jsx)(e,{})})]},p={args:{profiles:[],onSelect:d(),onCreate:d(),onDelete:d()}},m={args:{profiles:[s],onSelect:d(),onCreate:d(),onDelete:d()}},h={args:{profiles:[s,o,a],onSelect:d(),onCreate:d(),onDelete:d()}},g={args:{profiles:[s,o,a,{...s,id:`vp-4`,name:`Support Articles`},{...o,id:`vp-5`,name:`Social Media Posts`},{...a,id:`vp-6`,name:`Internal Wiki`}],onSelect:d(),onCreate:d(),onDelete:d()}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    profiles: [],
    onSelect: fn(),
    onCreate: fn(),
    onDelete: fn()
  }
}`,...p.parameters?.docs?.source},description:{story:`Empty state — no profiles yet.`,...p.parameters?.docs?.description}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    profiles: [sampleProfile],
    onSelect: fn(),
    onCreate: fn(),
    onDelete: fn()
  }
}`,...m.parameters?.docs?.source},description:{story:`Single profile.`,...m.parameters?.docs?.description}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    profiles: [sampleProfile, casualProfile, technicalProfile],
    onSelect: fn(),
    onCreate: fn(),
    onDelete: fn()
  }
}`,...h.parameters?.docs?.source},description:{story:`Multiple profiles in a grid.`,...h.parameters?.docs?.description}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    profiles: [sampleProfile, casualProfile, technicalProfile, {
      ...sampleProfile,
      id: "vp-4",
      name: "Support Articles"
    }, {
      ...casualProfile,
      id: "vp-5",
      name: "Social Media Posts"
    }, {
      ...technicalProfile,
      id: "vp-6",
      name: "Internal Wiki"
    }],
    onSelect: fn(),
    onCreate: fn(),
    onDelete: fn()
  }
}`,...g.parameters?.docs?.source},description:{story:`Many profiles to test grid scaling and search.`,...g.parameters?.docs?.description}}},_=[`Empty`,`SingleProfile`,`MultipleProfiles`,`ManyProfiles`]}))();export{p as Empty,g as ManyProfiles,h as MultipleProfiles,m as SingleProfile,_ as __namedExportsOrder,f as default};