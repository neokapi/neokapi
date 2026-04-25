import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{$ as n,t as r}from"./src-Bvgh6dl3.js";import{a as i,i as a}from"./TMFacetSidebar-BUB5Sjt9.js";var o,s,c,l,u,d;e((()=>{r(),o=t(),s={title:`Foundations/ScrollArea`,component:a,tags:[`autodocs`],decorators:[e=>(0,o.jsx)(`div`,{style:{maxWidth:350,padding:16},children:(0,o.jsx)(e,{})})]},c=[`English (en-US)`,`French (fr-FR)`,`German (de-DE)`,`Spanish (es-ES)`,`Italian (it-IT)`,`Portuguese (pt-BR)`,`Japanese (ja-JP)`,`Korean (ko-KR)`,`Chinese Simplified (zh-CN)`,`Chinese Traditional (zh-TW)`,`Arabic (ar-SA)`,`Russian (ru-RU)`,`Hindi (hi-IN)`,`Turkish (tr-TR)`,`Dutch (nl-NL)`],l={render:()=>(0,o.jsx)(a,{className:`h-48 w-full rounded-md border`,children:(0,o.jsxs)(`div`,{className:`p-4`,children:[(0,o.jsx)(`h4`,{className:`mb-4 text-sm font-medium leading-none`,children:`Target Locales`}),c.map(e=>(0,o.jsxs)(`div`,{children:[(0,o.jsx)(`div`,{className:`text-sm`,children:e}),(0,o.jsx)(n,{className:`my-2`})]},e))]})})},u={render:()=>(0,o.jsxs)(a,{className:`w-full whitespace-nowrap rounded-md border`,children:[(0,o.jsx)(`div`,{className:`flex w-max space-x-4 p-4`,children:c.map(e=>(0,o.jsx)(`div`,{className:`shrink-0 rounded-md border px-3 py-1.5 text-sm`,children:e},e))}),(0,o.jsx)(i,{orientation:`horizontal`})]})},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  render: () => <ScrollArea className="h-48 w-full rounded-md border">
      <div className="p-4">
        <h4 className="mb-4 text-sm font-medium leading-none">Target Locales</h4>
        {locales.map(locale => <div key={locale}>
            <div className="text-sm">{locale}</div>
            <Separator className="my-2" />
          </div>)}
      </div>
    </ScrollArea>
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  render: () => <ScrollArea className="w-full whitespace-nowrap rounded-md border">
      <div className="flex w-max space-x-4 p-4">
        {locales.map(locale => <div key={locale} className="shrink-0 rounded-md border px-3 py-1.5 text-sm">
            {locale}
          </div>)}
      </div>
      <ScrollBar orientation="horizontal" />
    </ScrollArea>
}`,...u.parameters?.docs?.source}}},d=[`Default`,`Horizontal`]}))();export{l as Default,u as Horizontal,d as __namedExportsOrder,s as default};