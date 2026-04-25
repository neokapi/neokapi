import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{nt as n,tt as r}from"./iframe-BJVhYAGS.js";var i,a,o,s,c,l;e((()=>{n(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o={title:`Brand/PersonalityTagPicker`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{maxWidth:480,padding:24},children:(0,i.jsx)(e,{})})]},s={args:{tags:[],onChange:a()}},c={args:{tags:[`friendly`,`professional`,`warm`],onChange:a()}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    tags: [],
    onChange: fn()
  }
}`,...s.parameters?.docs?.source},description:{story:`No tags selected — shows all suggested categories.`,...s.parameters?.docs?.description}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    tags: ["friendly", "professional", "warm"],
    onChange: fn()
  }
}`,...c.parameters?.docs?.source},description:{story:`Three tags already selected — some suggestions dimmed.`,...c.parameters?.docs?.description}}},l=[`Empty`,`WithTags`]}))();export{s as Empty,c as WithTags,l as __namedExportsOrder,o as default};