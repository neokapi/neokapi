import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{N as n,P as r}from"./iframe-BJVhYAGS.js";var i,a,o,s,c,l,u,d,f,p;e((()=>{r(),i=t(),a={title:`Billing/SubscriptionBadge`,component:n,tags:[`autodocs`]},o={args:{plan:`free`,status:`active`}},s={args:{plan:`pro`,status:`active`}},c={args:{plan:`team`,status:`trialing`}},l={args:{plan:`enterprise`,status:`past_due`}},u={args:{plan:`pro`,status:`canceled`}},d={args:{plan:`team`}},f={render:()=>(0,i.jsx)(`div`,{style:{display:`flex`,flexDirection:`column`,gap:12},children:[`free`,`pro`,`team`,`enterprise`].map(e=>[`active`,`trialing`,`past_due`,`canceled`].map(t=>(0,i.jsxs)(`div`,{style:{display:`flex`,alignItems:`center`,gap:8},children:[(0,i.jsxs)(`span`,{style:{width:140,fontSize:12,color:`#888`},children:[e,` / `,t]}),(0,i.jsx)(n,{plan:e,status:t})]},`${e}-${t}`)))})},o.parameters={...o.parameters,docs:{...o.parameters?.docs,source:{originalSource:`{
  args: {
    plan: "free",
    status: "active"
  }
}`,...o.parameters?.docs?.source}}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    plan: "pro",
    status: "active"
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    plan: "team",
    status: "trialing"
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    plan: "enterprise",
    status: "past_due"
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    plan: "pro",
    status: "canceled"
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    plan: "team"
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    display: "flex",
    flexDirection: "column",
    gap: 12
  }}>
      {(["free", "pro", "team", "enterprise"] as const).map(plan => (["active", "trialing", "past_due", "canceled"] as const).map(status => <div key={\`\${plan}-\${status}\`} style={{
      display: "flex",
      alignItems: "center",
      gap: 8
    }}>
            <span style={{
        width: 140,
        fontSize: 12,
        color: "#888"
      }}>
              {plan} / {status}
            </span>
            <SubscriptionBadge plan={plan} status={status} />
          </div>))}
    </div>
}`,...f.parameters?.docs?.source}}},p=[`FreeActive`,`ProActive`,`TeamTrialing`,`EnterprisePastDue`,`ProCanceled`,`PlanOnly`,`AllCombinations`]}))();export{f as AllCombinations,l as EnterprisePastDue,o as FreeActive,d as PlanOnly,s as ProActive,u as ProCanceled,c as TeamTrialing,p as __namedExportsOrder,a as default};