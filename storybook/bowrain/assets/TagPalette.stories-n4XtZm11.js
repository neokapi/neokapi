import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{t as n}from"./src-Bvgh6dl3.js";import{l as r}from"./TMGroupedEntry-CGOac2Ak.js";import{E as i,d as a,l as o,n as s,o as c,p as l,r as u,t as d,u as f,v as p,y as m}from"./fixtures-D8OXoGLB.js";var h,g,_,v,y,b,x,S,C,w;e((()=>{n(),o(),h=t(),{fn:g}=__STORYBOOK_MODULE_TEST__,_={title:`Editor/Tags/TagPalette`,component:r,tags:[`autodocs`],args:{onInsert:g()},decorators:[e=>(0,h.jsx)(`div`,{style:{maxWidth:600,padding:16},children:(0,h.jsx)(e,{})})]},v={args:{sourceSpans:[s,d]}},y={args:{sourceSpans:[s,d,a,f,m,p]}},b={args:{sourceSpans:[c,u,l]}},x={args:{sourceSpans:i}},S={args:{sourceSpans:[s,d,a,f],usedSpans:[s,d]}},C={args:{sourceSpans:[]}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
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
  args: {
    sourceSpans: [boldOpen, boldClose, italicOpen, italicClose],
    usedSpans: [boldOpen, boldClose]
  }
}`,...S.parameters?.docs?.source},description:{story:`Some tags already used — shown dimmed`,...S.parameters?.docs?.description}}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  args: {
    sourceSpans: []
  }
}`,...C.parameters?.docs?.source}}},w=[`BoldPair`,`MultiplePairs`,`WithCodeAndBreak`,`AllTags`,`PartiallyUsed`,`Empty`]}))();export{x as AllTags,v as BoldPair,C as Empty,y as MultiplePairs,S as PartiallyUsed,b as WithCodeAndBreak,w as __namedExportsOrder,_ as default};