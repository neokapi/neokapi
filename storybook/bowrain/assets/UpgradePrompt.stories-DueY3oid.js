import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{C as n,w as r}from"./iframe-Cr-Gz2Kj.js";var i,a,o,s,c,l,u,d,f;e((()=>{r(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o={title:`Billing/UpgradePrompt`,component:n,tags:[`autodocs`],args:{onUpgrade:a()}},s={args:{feature:`Git connectors`,minimumPlan:`pro`,currentPlan:`free`}},c={args:{feature:`@bravo code execution`,minimumPlan:`team`,currentPlan:`pro`}},l={args:{feature:`SSO/SAML`,minimumPlan:`enterprise`,currentPlan:`team`}},u={args:{feature:`API access`,minimumPlan:`pro`,currentPlan:`free`}},d={render:()=>(0,i.jsxs)(`div`,{style:{display:`flex`,flexDirection:`column`,gap:16,maxWidth:400},children:[(0,i.jsx)(n,{feature:`Git connectors`,minimumPlan:`pro`,currentPlan:`free`}),(0,i.jsx)(n,{feature:`@bravo code execution`,minimumPlan:`team`,currentPlan:`pro`}),(0,i.jsx)(n,{feature:`SSO/SAML`,minimumPlan:`enterprise`,currentPlan:`team`})]})},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    feature: "Git connectors",
    minimumPlan: "pro",
    currentPlan: "free"
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    feature: "@bravo code execution",
    minimumPlan: "team",
    currentPlan: "pro"
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    feature: "SSO/SAML",
    minimumPlan: "enterprise",
    currentPlan: "team"
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    feature: "API access",
    minimumPlan: "pro",
    currentPlan: "free"
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    display: "flex",
    flexDirection: "column",
    gap: 16,
    maxWidth: 400
  }}>
      <UpgradePrompt feature="Git connectors" minimumPlan="pro" currentPlan="free" />
      <UpgradePrompt feature="@bravo code execution" minimumPlan="team" currentPlan="pro" />
      <UpgradePrompt feature="SSO/SAML" minimumPlan="enterprise" currentPlan="team" />
    </div>
}`,...d.parameters?.docs?.source}}},f=[`GitConnectors`,`CodeExecution`,`SSOSAML`,`APIAccess`,`AllPrompts`]}))();export{u as APIAccess,d as AllPrompts,c as CodeExecution,s as GitConnectors,l as SSOSAML,f as __namedExportsOrder,o as default};