import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,r as a,t as o}from"./LocaleSelect-DQYOBlhw.js";import{n as s,r as c}from"./decorators-BptasVUO.js";var l,u,d,f,p,m,h;t((()=>{l=e(n(),1),a(),s(),u=r(),d={title:`Components/LocaleSelect`,component:o,tags:[`autodocs`],decorators:[c,e=>(0,u.jsx)(`div`,{style:{maxWidth:320,padding:16},children:(0,u.jsx)(e,{})})]},f={args:{value:`en-US`,onChange:()=>{}}},p={args:{value:``,onChange:()=>{},placeholder:`Choose a locale...`}},m={render:()=>{let[e,t]=(0,l.useState)([`fr-FR`,`de-DE`]);return(0,u.jsx)(i,{value:e,onChange:t})}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    value: "en-US",
    onChange: () => {}
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    value: "",
    onChange: () => {},
    placeholder: "Choose a locale..."
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [value, setValue] = useState(["fr-FR", "de-DE"]);
    return <MultiLocaleSelect value={value} onChange={setValue} />;
  }
}`,...m.parameters?.docs?.source}}},h=[`Default`,`WithPlaceholder`,`MultiSelect`]}))();export{f as Default,m as MultiSelect,p as WithPlaceholder,h as __namedExportsOrder,d as default};