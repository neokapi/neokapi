import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./FlowPage-_u-d039i.js";var i,a,o,s,c,l;e((()=>{n(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o={title:`Pages/FlowPage`,component:r,tags:[`autodocs`],args:{onChange:a(),onRun:a()},parameters:{layout:`fullscreen`},decorators:[e=>(0,i.jsx)(`div`,{style:{height:600},children:(0,i.jsx)(e,{})})]},s={args:{flowName:`translate`,flow:{steps:[{tool:`ai-translate`,config:{provider:`anthropic`}}]}}},c={args:{flowName:`new-flow`,flow:{steps:[]}}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    flowName: "translate",
    flow: {
      steps: [{
        tool: "ai-translate",
        config: {
          provider: "anthropic"
        }
      }]
    }
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    flowName: "new-flow",
    flow: {
      steps: []
    }
  }
}`,...c.parameters?.docs?.source}}},l=[`WithFlows`,`Empty`]}))();export{c as Empty,s as WithFlows,l as __namedExportsOrder,o as default};