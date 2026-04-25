import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{M as n,j as r}from"./iframe-Cr-Gz2Kj.js";var i,a,o,s,c,l,u,d,f,p;e((()=>{n(),i=t(),a={title:`Billing/UsageBar`,component:r,tags:[`autodocs`]},o=e=>new Date(Date.now()+e*24*60*60*1e3),s={args:{creditsUsed:12e4,creditsTotal:5e5,weekEnd:o(3)}},c={args:{creditsUsed:35e4,creditsTotal:5e5,weekEnd:o(2)}},l={args:{creditsUsed:45e4,creditsTotal:5e5,weekEnd:o(1)}},u={args:{creditsUsed:5e5,creditsTotal:5e5,weekEnd:o(4)}},d={args:{creditsUsed:3e4,creditsTotal:5e4,weekEnd:o(5)}},f={render:()=>(0,i.jsxs)(`div`,{style:{display:`flex`,flexDirection:`column`,gap:24,maxWidth:400},children:[(0,i.jsxs)(`div`,{children:[(0,i.jsx)(`div`,{style:{fontSize:12,marginBottom:4,color:`#888`},children:`Low (24%)`}),(0,i.jsx)(r,{creditsUsed:12e4,creditsTotal:5e5,weekEnd:o(3)})]}),(0,i.jsxs)(`div`,{children:[(0,i.jsx)(`div`,{style:{fontSize:12,marginBottom:4,color:`#888`},children:`Medium (70%)`}),(0,i.jsx)(r,{creditsUsed:35e4,creditsTotal:5e5,weekEnd:o(2)})]}),(0,i.jsxs)(`div`,{children:[(0,i.jsx)(`div`,{style:{fontSize:12,marginBottom:4,color:`#888`},children:`High (90%)`}),(0,i.jsx)(r,{creditsUsed:45e4,creditsTotal:5e5,weekEnd:o(1)})]}),(0,i.jsxs)(`div`,{children:[(0,i.jsx)(`div`,{style:{fontSize:12,marginBottom:4,color:`#888`},children:`Exhausted (100%)`}),(0,i.jsx)(r,{creditsUsed:5e5,creditsTotal:5e5,weekEnd:o(4)})]})]})},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    creditsUsed: 120_000,
    creditsTotal: 500_000,
    weekEnd: futureDate(3)
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    creditsUsed: 350_000,
    creditsTotal: 500_000,
    weekEnd: futureDate(2)
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    creditsUsed: 450_000,
    creditsTotal: 500_000,
    weekEnd: futureDate(1)
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    creditsUsed: 500_000,
    creditsTotal: 500_000,
    weekEnd: futureDate(4)
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    creditsUsed: 30_000,
    creditsTotal: 50_000,
    weekEnd: futureDate(5)
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    display: "flex",
    flexDirection: "column",
    gap: 24,
    maxWidth: 400
  }}>
      <div>
        <div style={{
        fontSize: 12,
        marginBottom: 4,
        color: "#888"
      }}>Low (24%)</div>
        <UsageBar creditsUsed={120_000} creditsTotal={500_000} weekEnd={futureDate(3)} />
      </div>
      <div>
        <div style={{
        fontSize: 12,
        marginBottom: 4,
        color: "#888"
      }}>Medium (70%)</div>
        <UsageBar creditsUsed={350_000} creditsTotal={500_000} weekEnd={futureDate(2)} />
      </div>
      <div>
        <div style={{
        fontSize: 12,
        marginBottom: 4,
        color: "#888"
      }}>High (90%)</div>
        <UsageBar creditsUsed={450_000} creditsTotal={500_000} weekEnd={futureDate(1)} />
      </div>
      <div>
        <div style={{
        fontSize: 12,
        marginBottom: 4,
        color: "#888"
      }}>Exhausted (100%)</div>
        <UsageBar creditsUsed={500_000} creditsTotal={500_000} weekEnd={futureDate(4)} />
      </div>
    </div>
}`,...f.parameters?.docs?.source}}},p=[`LowUsage`,`MediumUsage`,`HighUsage`,`Exhausted`,`FreeplanSmall`,`AllLevels`]}))();export{f as AllLevels,u as Exhausted,d as FreeplanSmall,l as HighUsage,s as LowUsage,c as MediumUsage,p as __namedExportsOrder,a as default};