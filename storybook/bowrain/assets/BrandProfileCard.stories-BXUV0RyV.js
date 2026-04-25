import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./BrandProfileCard-DS82YM5l.js";import{i,m as a,t as o,u as s}from"./fixtures-Cf-tGaH5.js";var c,l,u,d,f,p,m,h;e((()=>{n(),i(),c=t(),{fn:l}=__STORYBOOK_MODULE_TEST__,u={title:`Brand/BrandProfileCard`,component:r,tags:[`autodocs`],decorators:[e=>(0,c.jsx)(`div`,{style:{maxWidth:360,padding:24},children:(0,c.jsx)(e,{})})]},d={args:{profile:s,onClick:l(),onDelete:l()}},f={args:{profile:o,onClick:l()}},p={args:{profile:a,onClick:l(),onDelete:l()}},m={args:{profile:{...s,name:`Enterprise Documentation Style Guide for All Customer-Facing Content`,description:`This profile governs all customer-facing documentation including API references, getting started guides, tutorials, and troubleshooting articles. It enforces formal tone, active voice, and approved terminology across all locales.`},onClick:l(),onDelete:l()}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    profile: sampleProfile,
    onClick: fn(),
    onDelete: fn()
  }
}`,...d.parameters?.docs?.source},description:{story:`Formal enterprise profile with delete action.`,...d.parameters?.docs?.description}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    profile: casualProfile,
    onClick: fn()
  }
}`,...f.parameters?.docs?.source},description:{story:`Card without delete button (read-only).`,...f.parameters?.docs?.description}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    profile: technicalProfile,
    onClick: fn(),
    onDelete: fn()
  }
}`,...p.parameters?.docs?.source},description:{story:`Technical profile with fewer personality tags.`,...p.parameters?.docs?.description}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    profile: {
      ...sampleProfile,
      name: "Enterprise Documentation Style Guide for All Customer-Facing Content",
      description: "This profile governs all customer-facing documentation including API references, getting started guides, tutorials, and troubleshooting articles. It enforces formal tone, active voice, and approved terminology across all locales."
    },
    onClick: fn(),
    onDelete: fn()
  }
}`,...m.parameters?.docs?.source},description:{story:`Profile with a long name and description to test truncation.`,...m.parameters?.docs?.description}}},h=[`Default`,`WithoutDelete`,`Technical`,`LongContent`]}))();export{d as Default,m as LongContent,p as Technical,f as WithoutDelete,h as __namedExportsOrder,u as default};