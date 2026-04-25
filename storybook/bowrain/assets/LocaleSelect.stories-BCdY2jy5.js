import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,r as a,t as o}from"./locale-select-CN1b3mao.js";function s({initial:e=``,locales:t=d}){let[n,r]=(0,l.useState)(e);return(0,u.jsxs)(`div`,{className:`max-w-sm space-y-2`,children:[(0,u.jsx)(o,{value:n,onChange:r,locales:t}),(0,u.jsxs)(`pre`,{className:`rounded bg-muted p-2 font-mono text-xs`,children:[`value: `,JSON.stringify(n)]})]})}function c({initial:e=[],locales:t=d}){let[n,r]=(0,l.useState)(e);return(0,u.jsxs)(`div`,{className:`max-w-lg space-y-2`,children:[(0,u.jsx)(i,{value:n,onChange:r,locales:t}),(0,u.jsxs)(`pre`,{className:`rounded bg-muted p-2 font-mono text-xs`,children:[`value: `,JSON.stringify(n)]})]})}var l,u,d,f,p,m,h,g,_,v,y;t((()=>{l=e(n(),1),a(),u=r(),d=[{code:`ar`,displayName:`Arabic`},{code:`zh`,displayName:`Chinese`},{code:`cs`,displayName:`Czech`},{code:`da`,displayName:`Danish`},{code:`nl`,displayName:`Dutch`},{code:`en`,displayName:`English`},{code:`fi`,displayName:`Finnish`},{code:`fr`,displayName:`French`},{code:`de`,displayName:`German`},{code:`el`,displayName:`Greek`},{code:`it`,displayName:`Italian`},{code:`ja`,displayName:`Japanese`},{code:`ko`,displayName:`Korean`},{code:`nb`,displayName:`Norwegian Bokmål`},{code:`pl`,displayName:`Polish`},{code:`pt`,displayName:`Portuguese`},{code:`pt-BR`,displayName:`Brazilian Portuguese`},{code:`ru`,displayName:`Russian`},{code:`zh-Hans`,displayName:`Simplified Chinese`},{code:`es`,displayName:`Spanish`},{code:`sv`,displayName:`Swedish`},{code:`th`,displayName:`Thai`},{code:`zh-Hant`,displayName:`Traditional Chinese`},{code:`tr`,displayName:`Turkish`}],f={title:`Foundations/LocaleSelect`,component:o,tags:[`autodocs`],parameters:{docs:{description:{component:`Locale selector with autocomplete. Single-select for source language, multi-select with chips for target languages. Pure component — locales passed as props.`}}}},p={name:`Single Select`,render:()=>(0,u.jsx)(s,{initial:`en`})},m={name:`Single Select — Empty`,render:()=>(0,u.jsx)(s,{})},h={name:`Multi Select`,render:()=>(0,u.jsx)(c,{initial:[`fr`,`de`,`ja`]})},g={name:`Multi Select — Empty`,render:()=>(0,u.jsx)(c,{})},_={name:`Source + Target (Side by Side)`,render:()=>(0,u.jsxs)(`div`,{className:`max-w-2xl space-y-4`,children:[(0,u.jsxs)(`div`,{children:[(0,u.jsx)(`label`,{className:`mb-1 block text-xs text-muted-foreground`,children:`Source Language`}),(0,u.jsx)(s,{initial:`en`})]}),(0,u.jsxs)(`div`,{children:[(0,u.jsx)(`label`,{className:`mb-1 block text-xs text-muted-foreground`,children:`Target Languages`}),(0,u.jsx)(c,{initial:[`fr`,`de`,`pt-BR`]})]})]})},v={name:`POSIX-style Codes`,render:()=>(0,u.jsx)(c,{initial:[`pt_BR`,`zh_Hans`],locales:d.map(e=>({...e,code:e.code.replace(/-/g,`_`)}))})},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  name: "Single Select",
  render: () => <SingleWrapper initial="en" />
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "Single Select — Empty",
  render: () => <SingleWrapper />
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "Multi Select",
  render: () => <MultiWrapper initial={["fr", "de", "ja"]} />
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "Multi Select — Empty",
  render: () => <MultiWrapper />
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
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
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  name: "POSIX-style Codes",
  render: () => {
    const posixLocales: LocaleInfo[] = sampleLocales.map(l => ({
      ...l,
      code: l.code.replace(/-/g, "_")
    }));
    return <MultiWrapper initial={["pt_BR", "zh_Hans"]} locales={posixLocales} />;
  }
}`,...v.parameters?.docs?.source}}},y=[`SingleSelect`,`SingleSelectEmpty`,`MultiSelect`,`MultiSelectEmpty`,`SideBySide`,`POSIXCodes`]}))();export{h as MultiSelect,g as MultiSelectEmpty,v as POSIXCodes,_ as SideBySide,p as SingleSelect,m as SingleSelectEmpty,y as __namedExportsOrder,f as default};