import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{i as n,n as r,t as i}from"./schema-story-BnmQqeze.js";var a,o,s,c,l,u;e((()=>{n(),a=t(),o={title:`Formats & Tools/Formats/Document/Wiki Format`,parameters:{layout:`padded`}},s={name:`Native Configuration`,render:()=>(0,a.jsx)(r,{schemaName:`wiki`,source:`builtIn`})},c={name:`Okapi Configuration`,render:()=>(0,a.jsx)(r,{schemaName:`okf_wiki`,source:`bridge`})},l={name:`Side by Side`,render:()=>(0,a.jsx)(i,{nativeName:`wiki`,okapiName:`okf_wiki`})},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  name: "Native Configuration",
  render: () => <FormatConfig schemaName="wiki" source="builtIn" />
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  name: "Okapi Configuration",
  render: () => <FormatConfig schemaName="okf_wiki" source="bridge" />
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  name: "Side by Side",
  render: () => <FormatCompare nativeName="wiki" okapiName="okf_wiki" />
}`,...l.parameters?.docs?.source}}},u=[`NativeConfig`,`OkapiConfig`,`Compare`]}))();export{l as Compare,s as NativeConfig,c as OkapiConfig,u as __namedExportsOrder,o as default};