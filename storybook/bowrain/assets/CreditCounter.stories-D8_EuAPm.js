import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{A as n,k as r}from"./iframe-BJVhYAGS.js";var i,a,o,s,c,l,u,d;e((()=>{n(),i=t(),a={title:`Billing/CreditCounter`,component:r,tags:[`autodocs`]},o={args:{creditsUsed:77e3,creditsTotal:5e5}},s={args:{creditsUsed:45e4,creditsTotal:5e5}},c={args:{creditsUsed:77e3,creditsTotal:5e5,compact:!0}},l={args:{creditsUsed:45e4,creditsTotal:5e5,compact:!0}},u={render:()=>(0,i.jsxs)(`div`,{style:{display:`flex`,flexDirection:`column`,gap:16},children:[(0,i.jsxs)(`div`,{children:[(0,i.jsx)(`div`,{style:{fontSize:12,marginBottom:4,color:`#888`},children:`Default`}),(0,i.jsx)(r,{creditsUsed:77e3,creditsTotal:5e5})]}),(0,i.jsxs)(`div`,{children:[(0,i.jsx)(`div`,{style:{fontSize:12,marginBottom:4,color:`#888`},children:`Compact`}),(0,i.jsx)(r,{creditsUsed:77e3,creditsTotal:5e5,compact:!0})]}),(0,i.jsxs)(`div`,{children:[(0,i.jsx)(`div`,{style:{fontSize:12,marginBottom:4,color:`#888`},children:`Compact (high usage)`}),(0,i.jsx)(r,{creditsUsed:45e4,creditsTotal:5e5,compact:!0})]})]})},o.parameters={...o.parameters,docs:{...o.parameters?.docs,source:{originalSource:`{
  args: {
    creditsUsed: 77_000,
    creditsTotal: 500_000
  }
}`,...o.parameters?.docs?.source}}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    creditsUsed: 450_000,
    creditsTotal: 500_000
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    creditsUsed: 77_000,
    creditsTotal: 500_000,
    compact: true
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    creditsUsed: 450_000,
    creditsTotal: 500_000,
    compact: true
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    display: "flex",
    flexDirection: "column",
    gap: 16
  }}>
      <div>
        <div style={{
        fontSize: 12,
        marginBottom: 4,
        color: "#888"
      }}>Default</div>
        <CreditCounter creditsUsed={77_000} creditsTotal={500_000} />
      </div>
      <div>
        <div style={{
        fontSize: 12,
        marginBottom: 4,
        color: "#888"
      }}>Compact</div>
        <CreditCounter creditsUsed={77_000} creditsTotal={500_000} compact />
      </div>
      <div>
        <div style={{
        fontSize: 12,
        marginBottom: 4,
        color: "#888"
      }}>Compact (high usage)</div>
        <CreditCounter creditsUsed={450_000} creditsTotal={500_000} compact />
      </div>
    </div>
}`,...u.parameters?.docs?.source}}},d=[`Default`,`HighUsage`,`Compact`,`CompactHighUsage`,`Variants`]}))();export{c as Compact,l as CompactHighUsage,o as Default,s as HighUsage,u as Variants,d as __namedExportsOrder,a as default};