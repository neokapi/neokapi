import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./TermStatusBadge-D8rfnzxm.js";var i,a,o,s,c,l,u,d,f,p;e((()=>{n(),i=t(),a={title:`Resource Browser/TermStatusBadge`,component:r,tags:[`autodocs`],parameters:{docs:{description:{component:`Semantic status badge for term lifecycle status. Uses OKLCH color space with CSS custom properties for dark mode. Deprecated and forbidden statuses display with a strikethrough.`}}}},o={args:{status:`preferred`}},s={args:{status:`approved`}},c={args:{status:`admitted`}},l={args:{status:`proposed`}},u={args:{status:`deprecated`}},d={args:{status:`forbidden`}},f={render:()=>(0,i.jsxs)(`div`,{className:`flex items-center gap-2`,children:[(0,i.jsx)(r,{status:`preferred`}),(0,i.jsx)(r,{status:`approved`}),(0,i.jsx)(r,{status:`admitted`}),(0,i.jsx)(r,{status:`proposed`}),(0,i.jsx)(r,{status:`deprecated`}),(0,i.jsx)(r,{status:`forbidden`})]})},o.parameters={...o.parameters,docs:{...o.parameters?.docs,source:{originalSource:`{
  args: {
    status: "preferred"
  }
}`,...o.parameters?.docs?.source}}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    status: "approved"
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    status: "admitted"
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    status: "proposed"
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    status: "deprecated"
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    status: "forbidden"
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => <div className="flex items-center gap-2">
      <TermStatusBadge status="preferred" />
      <TermStatusBadge status="approved" />
      <TermStatusBadge status="admitted" />
      <TermStatusBadge status="proposed" />
      <TermStatusBadge status="deprecated" />
      <TermStatusBadge status="forbidden" />
    </div>
}`,...f.parameters?.docs?.source},description:{story:`All six term statuses shown together for comparison.`,...f.parameters?.docs?.description}}},p=[`Preferred`,`Approved`,`Admitted`,`Proposed`,`Deprecated`,`Forbidden`,`AllStatuses`]}))();export{c as Admitted,f as AllStatuses,s as Approved,u as Deprecated,d as Forbidden,o as Preferred,l as Proposed,p as __namedExportsOrder,a as default};