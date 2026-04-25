import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{dt as n,ft as r,lt as i,mt as a,pt as o,t as s,ut as c}from"./src-Bvgh6dl3.js";var l,u,d,f,p,m;e((()=>{s(),l=t(),u={title:`Foundations/Combobox`,tags:[`autodocs`],decorators:[e=>(0,l.jsx)(`div`,{style:{maxWidth:320,padding:16},children:(0,l.jsx)(e,{})})]},d=[{value:`en-US`,label:`English (US)`},{value:`fr-FR`,label:`French (France)`},{value:`de-DE`,label:`German (Germany)`},{value:`ja-JP`,label:`Japanese (Japan)`},{value:`zh-CN`,label:`Chinese (Simplified)`}],f={render:()=>(0,l.jsxs)(i,{children:[(0,l.jsx)(r,{placeholder:`Search locales...`}),(0,l.jsx)(c,{children:(0,l.jsxs)(a,{children:[(0,l.jsx)(n,{children:`No locales found.`}),d.map(e=>(0,l.jsx)(o,{value:e.value,children:e.label},e.value))]})})]})},p={render:()=>(0,l.jsxs)(i,{children:[(0,l.jsx)(r,{placeholder:`Search locales...`,showClear:!0}),(0,l.jsx)(c,{children:(0,l.jsxs)(a,{children:[(0,l.jsx)(n,{children:`No locales found.`}),d.map(e=>(0,l.jsx)(o,{value:e.value,children:e.label},e.value))]})})]})},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => <Combobox>
      <ComboboxInput placeholder="Search locales..." />
      <ComboboxContent>
        <ComboboxList>
          <ComboboxEmpty>No locales found.</ComboboxEmpty>
          {locales.map(locale => <ComboboxItem key={locale.value} value={locale.value}>
              {locale.label}
            </ComboboxItem>)}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  render: () => <Combobox>
      <ComboboxInput placeholder="Search locales..." showClear />
      <ComboboxContent>
        <ComboboxList>
          <ComboboxEmpty>No locales found.</ComboboxEmpty>
          {locales.map(locale => <ComboboxItem key={locale.value} value={locale.value}>
              {locale.label}
            </ComboboxItem>)}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
}`,...p.parameters?.docs?.source}}},m=[`Default`,`WithClear`]}))();export{f as Default,p as WithClear,m as __namedExportsOrder,u as default};