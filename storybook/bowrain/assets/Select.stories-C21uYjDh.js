import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{t as n}from"./src-Q72DuJW2.js";import{a as r,i,n as a,o,r as s,s as c,t as l}from"./select-BYYGJvgy.js";var u,d,f,p,m,h;e((()=>{n(),u=t(),d={title:`Foundations/Select`,component:l,tags:[`autodocs`],decorators:[e=>(0,u.jsx)(`div`,{style:{maxWidth:280,padding:16},children:(0,u.jsx)(e,{})})]},f={render:()=>(0,u.jsxs)(l,{children:[(0,u.jsx)(o,{children:(0,u.jsx)(c,{placeholder:`Select a locale`})}),(0,u.jsx)(a,{children:(0,u.jsxs)(s,{children:[(0,u.jsx)(r,{children:`Locales`}),(0,u.jsx)(i,{value:`en-US`,children:`English (US)`}),(0,u.jsx)(i,{value:`fr-FR`,children:`French (France)`}),(0,u.jsx)(i,{value:`de-DE`,children:`German (Germany)`}),(0,u.jsx)(i,{value:`ja-JP`,children:`Japanese (Japan)`})]})})]})},p={render:()=>(0,u.jsxs)(l,{children:[(0,u.jsx)(o,{size:`sm`,children:(0,u.jsx)(c,{placeholder:`Format`})}),(0,u.jsxs)(a,{children:[(0,u.jsx)(i,{value:`json`,children:`JSON`}),(0,u.jsx)(i,{value:`xliff`,children:`XLIFF`}),(0,u.jsx)(i,{value:`po`,children:`PO`}),(0,u.jsx)(i,{value:`properties`,children:`Properties`})]})]})},m={render:()=>(0,u.jsxs)(l,{disabled:!0,children:[(0,u.jsx)(o,{children:(0,u.jsx)(c,{placeholder:`Disabled`})}),(0,u.jsx)(a,{children:(0,u.jsx)(i,{value:`a`,children:`Option A`})})]})},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => <Select>
      <SelectTrigger>
        <SelectValue placeholder="Select a locale" />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectLabel>Locales</SelectLabel>
          <SelectItem value="en-US">English (US)</SelectItem>
          <SelectItem value="fr-FR">French (France)</SelectItem>
          <SelectItem value="de-DE">German (Germany)</SelectItem>
          <SelectItem value="ja-JP">Japanese (Japan)</SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  render: () => <Select>
      <SelectTrigger size="sm">
        <SelectValue placeholder="Format" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="json">JSON</SelectItem>
        <SelectItem value="xliff">XLIFF</SelectItem>
        <SelectItem value="po">PO</SelectItem>
        <SelectItem value="properties">Properties</SelectItem>
      </SelectContent>
    </Select>
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => <Select disabled>
      <SelectTrigger>
        <SelectValue placeholder="Disabled" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="a">Option A</SelectItem>
      </SelectContent>
    </Select>
}`,...m.parameters?.docs?.source}}},h=[`Default`,`Small`,`Disabled`]}))();export{f as Default,m as Disabled,p as Small,h as __namedExportsOrder,d as default};