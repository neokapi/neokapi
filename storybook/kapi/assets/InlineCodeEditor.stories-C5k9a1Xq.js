import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./InlineCodeEditor-BXUyoH8j.js";import{a as i,g as a,h as o,l as s,m as c,p as l,u}from"./fixtures-BWPB7NV2.js";var d,f,p,m,h,g,_,v,y;e((()=>{n(),i(),d=t(),{fn:f}=__STORYBOOK_MODULE_TEST__,p={title:`Editor/Core/InlineCodeEditor`,component:r,tags:[`autodocs`],decorators:[e=>(0,d.jsx)(`div`,{style:{maxWidth:640,padding:16},children:(0,d.jsx)(e,{})})],parameters:{docs:{description:{component:`Lexical-based rich text editor for translatable content with visual inline code chips. Renders formatting (bold, italic, links) as styled tag chips. Enter saves, Escape cancels. Ctrl+1..9 inserts tags from the palette.`}}}},m={args:{initialCodedText:``,initialSpans:[],sourceSpans:a,onSave:f(),onCancel:f()}},h={args:{initialCodedText:o,initialSpans:a,sourceSpans:a,onSave:f(),onCancel:f()}},g={args:{initialCodedText:s,initialSpans:u,sourceSpans:u,onSave:f(),onCancel:f()}},_={args:{initialCodedText:l,initialSpans:c,sourceSpans:c,onSave:f(),onCancel:f()}},v={name:`Compact (no palette/preview)`,args:{initialCodedText:o,initialSpans:a,sourceSpans:a,onSave:f(),onCancel:f(),compact:!0}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    initialCodedText: "",
    initialSpans: [],
    sourceSpans: simpleBoldSpans,
    onSave: fn(),
    onCancel: fn()
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    initialCodedText: simpleBoldCodedText,
    initialSpans: simpleBoldSpans,
    sourceSpans: simpleBoldSpans,
    onSave: fn(),
    onCancel: fn()
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    initialCodedText: linkAndItalicCodedText,
    initialSpans: linkAndItalicSpans,
    sourceSpans: linkAndItalicSpans,
    onSave: fn(),
    onCancel: fn()
  }
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  args: {
    initialCodedText: richCodedText,
    initialSpans: richSpans,
    sourceSpans: richSpans,
    onSave: fn(),
    onCancel: fn()
  }
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  name: "Compact (no palette/preview)",
  args: {
    initialCodedText: simpleBoldCodedText,
    initialSpans: simpleBoldSpans,
    sourceSpans: simpleBoldSpans,
    onSave: fn(),
    onCancel: fn(),
    compact: true
  }
}`,...v.parameters?.docs?.source}}},y=[`Empty`,`WithBoldText`,`WithLinks`,`RichContent`,`Compact`]}))();export{v as Compact,m as Empty,_ as RichContent,h as WithBoldText,g as WithLinks,y as __namedExportsOrder,p as default};