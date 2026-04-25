import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./TagPalette-BDwQnE91.js";import{a as i,c as a,d as o,f as s,i as c,m as l,n as u,o as d,r as f,s as p,t as m}from"./fixtures-BWPB7NV2.js";var h,g,_,v,y,b,x,S,C,w;e((()=>{n(),i(),h=t(),{fn:g}=__STORYBOOK_MODULE_TEST__,_={title:`Editor/Tags/TagPalette`,component:r,tags:[`autodocs`],args:{onInsert:g()},decorators:[e=>(0,h.jsx)(`div`,{style:{maxWidth:600,padding:16},children:(0,h.jsx)(e,{})})]},v={args:{sourceSpans:[u,m]}},y={args:{sourceSpans:[u,m,p,d,s,o]}},b={args:{sourceSpans:[c,f,a]}},x={args:{sourceSpans:l}},S={name:`Partially used (dimmed)`,args:{sourceSpans:[u,m,p,d],usedSpans:[u,m]}},C={args:{sourceSpans:[]}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  args: {
    sourceSpans: [boldOpen, boldClose]
  }
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  args: {
    sourceSpans: [boldOpen, boldClose, italicOpen, italicClose, linkOpen, linkClose]
  }
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  args: {
    sourceSpans: [codeOpen, codeClose, lineBreak]
  }
}`,...b.parameters?.docs?.source}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  args: {
    sourceSpans: richSpans
  }
}`,...x.parameters?.docs?.source}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  name: "Partially used (dimmed)",
  args: {
    sourceSpans: [boldOpen, boldClose, italicOpen, italicClose],
    usedSpans: [boldOpen, boldClose]
  }
}`,...S.parameters?.docs?.source}}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  args: {
    sourceSpans: []
  }
}`,...C.parameters?.docs?.source}}},w=[`BoldPair`,`MultiplePairs`,`WithCodeAndBreak`,`AllTags`,`PartiallyUsed`,`Empty`]}))();export{x as AllTags,v as BoldPair,C as Empty,y as MultiplePairs,S as PartiallyUsed,b as WithCodeAndBreak,w as __namedExportsOrder,_ as default};