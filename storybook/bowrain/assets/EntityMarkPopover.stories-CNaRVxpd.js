import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./EntityMarkPopover-BWaJvnnD.js";var i,a,o,s,c,l;e((()=>{n(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o={title:`Editor/Entities/EntityMarkPopover`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{position:`relative`,padding:80},children:(0,i.jsx)(e,{})})]},s={args:{text:`John Smith`,start:0,end:10,position:{x:100,y:40},onConfirm:a(),onCancel:a()}},c={args:{text:`Acme Corporation International`,start:15,end:45,position:{x:200,y:60},onConfirm:a(),onCancel:a()}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    text: "John Smith",
    start: 0,
    end: 10,
    position: {
      x: 100,
      y: 40
    },
    onConfirm: fn(),
    onCancel: fn()
  }
}`,...s.parameters?.docs?.source},description:{story:`Mark a person entity.`,...s.parameters?.docs?.description}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    text: "Acme Corporation International",
    start: 15,
    end: 45,
    position: {
      x: 200,
      y: 60
    },
    onConfirm: fn(),
    onCancel: fn()
  }
}`,...c.parameters?.docs?.source},description:{story:`Mark a longer organization name.`,...c.parameters?.docs?.description}}},l=[`Default`,`Organization`]}))();export{s as Default,c as Organization,l as __namedExportsOrder,o as default};