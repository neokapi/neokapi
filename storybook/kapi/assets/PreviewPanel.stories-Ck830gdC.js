import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./PreviewPanel-DWvA47yi.js";var i,a,o,s,c,l,u;e((()=>{n(),i=t(),a=new Map([[`tool-0`,`ai-translate`],[`tool-1`,`qa-check`]]),o={parts:{p1:{initial:{id:`p1`,type:`Block`,summary:`Hello world`,sourceText:`Hello world`},afterNode:{"tool-0":{id:`p1`,type:`Block`,summary:`Hello world`,sourceText:`Hello world`,targetText:`Bonjour le monde`},"tool-1":{id:`p1`,type:`Block`,summary:`Hello world`,sourceText:`Hello world`,targetText:`Bonjour le monde`}}}},nodeOrder:[`tool-0`,`tool-1`]},s={title:`Flow Editor/PreviewPanel`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{width:700},children:(0,i.jsx)(e,{})})]},c={args:{onPreview:async()=>o,sourceLang:`en-US`,targetLang:`fr-FR`,nodeNames:a}},l={args:{onPreview:async()=>({parts:{},nodeOrder:[]}),sourceLang:`en-US`,targetLang:`fr-FR`,nodeNames:new Map}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    onPreview: async () => mockResult,
    sourceLang: "en-US",
    targetLang: "fr-FR",
    nodeNames
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    onPreview: async () => ({
      parts: {},
      nodeOrder: []
    }),
    sourceLang: "en-US",
    targetLang: "fr-FR",
    nodeNames: new Map()
  }
}`,...l.parameters?.docs?.source}}},u=[`WithSampleText`,`Empty`]}))();export{l as Empty,c as WithSampleText,u as __namedExportsOrder,s as default};