import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{n as o,r as s,t as c}from"./locale-select-BnmiVczV.js";function l({initial:e=``,locales:t=p}){let[n,r]=(0,d.useState)(e);return(0,f.jsxs)(`div`,{className:`max-w-sm space-y-2`,children:[(0,f.jsx)(c,{value:n,onChange:r,locales:t}),(0,f.jsxs)(`pre`,{className:`rounded bg-muted p-2 font-mono text-xs`,children:[`value: `,JSON.stringify(n)]})]})}function u({initial:e=[],locales:t=p}){let[n,r]=(0,d.useState)(e);return(0,f.jsxs)(`div`,{className:`max-w-lg space-y-2`,children:[(0,f.jsx)(o,{value:n,onChange:r,locales:t}),(0,f.jsxs)(`pre`,{className:`rounded bg-muted p-2 font-mono text-xs`,children:[`value: `,JSON.stringify(n)]})]})}var d,f,p,m,h,g,_,v,y,b,x;t((()=>{i(),d=e(n(),1),s(),f=r(),p=[{code:`ar`,displayName:`Arabic`},{code:`zh`,displayName:`Chinese`},{code:`cs`,displayName:`Czech`},{code:`da`,displayName:`Danish`},{code:`nl`,displayName:`Dutch`},{code:`en`,displayName:`English`},{code:`fi`,displayName:`Finnish`},{code:`fr`,displayName:`French`},{code:`de`,displayName:`German`},{code:`el`,displayName:`Greek`},{code:`it`,displayName:`Italian`},{code:`ja`,displayName:`Japanese`},{code:`ko`,displayName:`Korean`},{code:`nb`,displayName:`Norwegian Bokmål`},{code:`pl`,displayName:`Polish`},{code:`pt`,displayName:`Portuguese`},{code:`pt-BR`,displayName:`Brazilian Portuguese`},{code:`ru`,displayName:`Russian`},{code:`zh-Hans`,displayName:`Simplified Chinese`},{code:`es`,displayName:`Spanish`},{code:`sv`,displayName:`Swedish`},{code:`th`,displayName:`Thai`},{code:`zh-Hant`,displayName:`Traditional Chinese`},{code:`tr`,displayName:`Turkish`}],m={title:`Foundations/LocaleSelect`,component:c,tags:[`autodocs`],parameters:{docs:{description:{component:`Locale selector with autocomplete. Single-select for source language, multi-select with chips for target languages. Pure component — locales passed as props.`}}}},h={name:`Single Select`,render:()=>(0,f.jsx)(l,{initial:`en`})},g={name:`Single Select — Empty`,render:()=>(0,f.jsx)(l,{})},_={name:`Multi Select`,render:()=>(0,f.jsx)(u,{initial:[`fr`,`de`,`ja`]})},v={name:`Multi Select — Empty`,render:()=>(0,f.jsx)(u,{})},y={name:`Source + Target (Side by Side)`,render:()=>(0,f.jsxs)(`div`,{className:`max-w-2xl space-y-4`,children:[(0,f.jsxs)(`div`,{children:[(0,f.jsx)(`label`,{className:`mb-1 block text-xs text-muted-foreground`,children:a(`3WxUkF`,`Source Language`)}),(0,f.jsx)(l,{initial:`en`})]}),(0,f.jsxs)(`div`,{children:[(0,f.jsx)(`label`,{className:`mb-1 block text-xs text-muted-foreground`,children:a(`4coIc9`,`Target Languages`)}),(0,f.jsx)(u,{initial:[`fr`,`de`,`pt-BR`]})]})]})},b={name:`POSIX-style Codes`,render:()=>(0,f.jsx)(u,{initial:[`pt_BR`,`zh_Hans`],locales:p.map(e=>({...e,code:e.code.replace(/-/g,`_`)}))})},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "Single Select",
  render: () => <SingleWrapper initial="en" />
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "Single Select — Empty",
  render: () => <SingleWrapper />
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "Multi Select",
  render: () => <MultiWrapper initial={["fr", "de", "ja"]} />
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  name: "Multi Select — Empty",
  render: () => <MultiWrapper />
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  name: "Source + Target (Side by Side)",
  render: () => <div className="max-w-2xl space-y-4">
      <div>
        <label className="mb-1 block text-xs text-muted-foreground">Source Language</label>
        <SingleWrapper initial="en" />
      </div>
      <div>
        <label className="mb-1 block text-xs text-muted-foreground">Target Languages</label>
        <MultiWrapper initial={["fr", "de", "pt-BR"]} />
      </div>
    </div>
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  name: "POSIX-style Codes",
  render: () => {
    const posixLocales: LocaleInfo[] = sampleLocales.map(l => ({
      ...l,
      code: l.code.replace(/-/g, "_")
    }));
    return <MultiWrapper initial={["pt_BR", "zh_Hans"]} locales={posixLocales} />;
  }
}`,...b.parameters?.docs?.source}}},x=[`SingleSelect`,`SingleSelectEmpty`,`MultiSelect`,`MultiSelectEmpty`,`SideBySide`,`POSIXCodes`]}))();export{_ as MultiSelect,v as MultiSelectEmpty,b as POSIXCodes,y as SideBySide,h as SingleSelect,g as SingleSelectEmpty,x as __namedExportsOrder,m as default};