import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{D as n,O as r}from"./iframe-Cr-Gz2Kj.js";var i,a,o,s,c,l,u,d,f,p;e((()=>{r(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o={title:`Billing/PlanCard`,component:n,tags:[`autodocs`],args:{onSelect:a()}},s={args:{plan:`free`,name:`Free`,price:`$0`,description:`Get started with AI-powered localization`,credits:`50K credits / week`,features:[{label:`@bravo chat (5 messages/day)`,included:!0},{label:`1 project`,included:!0},{label:`Git connectors`,included:!1},{label:`API access`,included:!1},{label:`Custom MT providers`,included:!1}]}},c={args:{plan:`pro`,name:`Pro`,price:`$25`,period:`mo`,description:`For professionals and small teams`,credits:`500K credits / week`,recommended:!0,features:[{label:`@bravo unlimited messages`,included:!0},{label:`Up to 10 projects`,included:!0},{label:`3 seats`,included:!0},{label:`Git connectors`,included:!0},{label:`API access`,included:!0},{label:`Custom MT providers`,included:!0},{label:`@bravo code execution`,included:!1}]}},l={args:{plan:`team`,name:`Team`,price:`$20`,period:`seat/mo`,description:`For growing teams`,credits:`2M credits / week`,features:[{label:`Everything in Pro`,included:!0},{label:`Unlimited projects`,included:!0},{label:`Unlimited seats`,included:!0},{label:`@bravo code execution`,included:!0},{label:`Custom connectors`,included:!0},{label:`SSO/SAML`,included:!1}]}},u={args:{plan:`enterprise`,name:`Enterprise`,price:`Custom`,description:`For large organizations`,credits:`Custom credit allocation`,ctaLabel:`Contact Sales`,features:[{label:`Everything in Team`,included:!0},{label:`SSO/SAML`,included:!0},{label:`Dedicated support`,included:!0},{label:`Custom agreements`,included:!0},{label:`SLA guarantees`,included:!0}]}},d={args:{plan:`pro`,name:`Pro`,price:`$25`,period:`mo`,credits:`500K credits / week`,current:!0,features:[{label:`@bravo unlimited messages`,included:!0},{label:`Up to 10 projects`,included:!0},{label:`3 seats`,included:!0}]}},f={render:()=>(0,i.jsxs)(`div`,{style:{display:`grid`,gridTemplateColumns:`repeat(4, 1fr)`,gap:16,maxWidth:1200},children:[(0,i.jsx)(n,{plan:`free`,name:`Free`,price:`$0`,credits:`50K credits / week`,features:[{label:`@bravo chat`,included:!0},{label:`1 project`,included:!0},{label:`Git connectors`,included:!1}]}),(0,i.jsx)(n,{plan:`pro`,name:`Pro`,price:`$25`,period:`mo`,credits:`500K credits / week`,recommended:!0,features:[{label:`Unlimited @bravo`,included:!0},{label:`10 projects`,included:!0},{label:`Git connectors`,included:!0},{label:`API access`,included:!0}]}),(0,i.jsx)(n,{plan:`team`,name:`Team`,price:`$20`,period:`seat/mo`,credits:`2M credits / week`,features:[{label:`Everything in Pro`,included:!0},{label:`Unlimited seats`,included:!0},{label:`Code execution`,included:!0}]}),(0,i.jsx)(n,{plan:`enterprise`,name:`Enterprise`,price:`Custom`,credits:`Custom`,ctaLabel:`Contact Sales`,features:[{label:`Everything in Team`,included:!0},{label:`SSO/SAML`,included:!0},{label:`Dedicated support`,included:!0}]})]})},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    plan: "free",
    name: "Free",
    price: "$0",
    description: "Get started with AI-powered localization",
    credits: "50K credits / week",
    features: [{
      label: "@bravo chat (5 messages/day)",
      included: true
    }, {
      label: "1 project",
      included: true
    }, {
      label: "Git connectors",
      included: false
    }, {
      label: "API access",
      included: false
    }, {
      label: "Custom MT providers",
      included: false
    }]
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    plan: "pro",
    name: "Pro",
    price: "$25",
    period: "mo",
    description: "For professionals and small teams",
    credits: "500K credits / week",
    recommended: true,
    features: [{
      label: "@bravo unlimited messages",
      included: true
    }, {
      label: "Up to 10 projects",
      included: true
    }, {
      label: "3 seats",
      included: true
    }, {
      label: "Git connectors",
      included: true
    }, {
      label: "API access",
      included: true
    }, {
      label: "Custom MT providers",
      included: true
    }, {
      label: "@bravo code execution",
      included: false
    }]
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    plan: "team",
    name: "Team",
    price: "$20",
    period: "seat/mo",
    description: "For growing teams",
    credits: "2M credits / week",
    features: [{
      label: "Everything in Pro",
      included: true
    }, {
      label: "Unlimited projects",
      included: true
    }, {
      label: "Unlimited seats",
      included: true
    }, {
      label: "@bravo code execution",
      included: true
    }, {
      label: "Custom connectors",
      included: true
    }, {
      label: "SSO/SAML",
      included: false
    }]
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    plan: "enterprise",
    name: "Enterprise",
    price: "Custom",
    description: "For large organizations",
    credits: "Custom credit allocation",
    ctaLabel: "Contact Sales",
    features: [{
      label: "Everything in Team",
      included: true
    }, {
      label: "SSO/SAML",
      included: true
    }, {
      label: "Dedicated support",
      included: true
    }, {
      label: "Custom agreements",
      included: true
    }, {
      label: "SLA guarantees",
      included: true
    }]
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    plan: "pro",
    name: "Pro",
    price: "$25",
    period: "mo",
    credits: "500K credits / week",
    current: true,
    features: [{
      label: "@bravo unlimited messages",
      included: true
    }, {
      label: "Up to 10 projects",
      included: true
    }, {
      label: "3 seats",
      included: true
    }]
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    display: "grid",
    gridTemplateColumns: "repeat(4, 1fr)",
    gap: 16,
    maxWidth: 1200
  }}>
      <PlanCard plan="free" name="Free" price="$0" credits="50K credits / week" features={[{
      label: "@bravo chat",
      included: true
    }, {
      label: "1 project",
      included: true
    }, {
      label: "Git connectors",
      included: false
    }]} />
      <PlanCard plan="pro" name="Pro" price="$25" period="mo" credits="500K credits / week" recommended features={[{
      label: "Unlimited @bravo",
      included: true
    }, {
      label: "10 projects",
      included: true
    }, {
      label: "Git connectors",
      included: true
    }, {
      label: "API access",
      included: true
    }]} />
      <PlanCard plan="team" name="Team" price="$20" period="seat/mo" credits="2M credits / week" features={[{
      label: "Everything in Pro",
      included: true
    }, {
      label: "Unlimited seats",
      included: true
    }, {
      label: "Code execution",
      included: true
    }]} />
      <PlanCard plan="enterprise" name="Enterprise" price="Custom" credits="Custom" ctaLabel="Contact Sales" features={[{
      label: "Everything in Team",
      included: true
    }, {
      label: "SSO/SAML",
      included: true
    }, {
      label: "Dedicated support",
      included: true
    }]} />
    </div>
}`,...f.parameters?.docs?.source}}},p=[`Free`,`ProRecommended`,`Team`,`Enterprise`,`CurrentPlan`,`AllPlans`]}))();export{f as AllPlans,d as CurrentPlan,u as Enterprise,s as Free,c as ProRecommended,l as Team,p as __namedExportsOrder,o as default};