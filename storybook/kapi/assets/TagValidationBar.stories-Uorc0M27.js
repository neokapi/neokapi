import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./TagValidationBar-DPSDDouz.js";var i,a,o,s,c,l,u,d,f,p,m,h;e((()=>{n(),i=t(),a={title:`Editor/Tags/TagValidationBar`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{maxWidth:500,padding:16},children:(0,i.jsx)(e,{})})]},o={valid:!0,errors:[],warnings:[]},s={valid:!1,errors:[{type:`missing_tag`,message:`Missing 1 opening "b" tag in target`},{type:`unpaired`,message:`Unpaired closing tag found`}],warnings:[]},c={valid:!0,errors:[],warnings:[{type:`extra_tag`,message:`Extra 1 closing "i" tag in target`}]},l={valid:!1,errors:[{type:`missing_tag`,message:`Missing "b" opening tag`}],warnings:[{type:`extra_tag`,message:`Extra "code" closing tag`}]},u={args:{validation:o}},d={args:{validation:s}},f={args:{validation:c}},p={args:{validation:l}},m={args:{validation:null}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    validation: noIssues
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    validation: withErrors
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    validation: withWarnings
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    validation: mixed
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    validation: null
  }
}`,...m.parameters?.docs?.source}}},h=[`NoIssues`,`Errors`,`Warnings`,`Mixed`,`Null`]}))();export{d as Errors,p as Mixed,u as NoIssues,m as Null,f as Warnings,h as __namedExportsOrder,a as default};