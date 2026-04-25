import{n as e}from"./chunk-DnJy8xQt.js";import{n as t,t as n}from"./CodedTextDisplay-QRTxMUm1.js";var r,i,a,o,s,c,l,u,d;e((()=>{t(),r={title:`Resource Browser/CodedTextDisplay`,component:n,tags:[`autodocs`],parameters:{docs:{description:{component:`Renders localization text with inline codes displayed as tag chips. Falls back to plain text when no coded text or spans are provided.`}}}},i={args:{text:`Welcome to the application`}},a=[{span_type:`opening`,type:`fmt:bold`,id:`1`,data:`<b>`},{span_type:`closing`,type:`fmt:bold`,id:`1`,data:`</b>`}],o={args:{text:`Click here to continue`,codedText:`Click here to continue`,spans:a}},s=[{span_type:`placeholder`,type:`entity:person`,id:`e1`,data:`Alice`}],c={args:{text:`Alice is a contributor`,codedText:` is a contributor`,spans:s}},l=[{span_type:`placeholder`,type:`entity:person`,id:`e1`,data:`Bob`},{span_type:`opening`,type:`fmt:bold`,id:`2`,data:`<strong>`},{span_type:`closing`,type:`fmt:bold`,id:`2`,data:`</strong>`},{span_type:`placeholder`,type:`entity:number`,id:`e2`,data:`42`}],u={args:{text:`Bob has completed 42 tasks successfully`,codedText:` has completed  tasks successfully`,spans:l}},i.parameters={...i.parameters,docs:{...i.parameters?.docs,source:{originalSource:`{
  args: {
    text: "Welcome to the application"
  }
}`,...i.parameters?.docs?.source}}},o.parameters={...o.parameters,docs:{...o.parameters?.docs,source:{originalSource:`{
  args: {
    text: "Click here to continue",
    codedText: "Click \\uE001here\\uE002 to continue",
    spans: boldSpans
  }
}`,...o.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    text: "Alice is a contributor",
    codedText: "\\uE003 is a contributor",
    spans: placeholderSpans
  }
}`,...c.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    text: "Bob has completed 42 tasks successfully",
    codedText: "\\uE003 has \\uE001completed\\uE002 \\uE003 tasks successfully",
    spans: mixedSpans
  }
}`,...u.parameters?.docs?.source}}},d=[`PlainText`,`BoldAndItalic`,`Placeholders`,`MixedContent`]}))();export{o as BoldAndItalic,u as MixedContent,c as Placeholders,i as PlainText,d as __namedExportsOrder,r as default};