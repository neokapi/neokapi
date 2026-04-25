import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./react-Baqbuk-D.js";import{t as n}from"./jsx-runtime-B-hFyic3.js";import{C as r,D as i,E as a,O as o,S as s,T as c,_ as l,a as u,b as d,c as f,d as p,f as m,g as h,h as g,i as _,k as v,l as y,m as b,n as x,o as S,p as C,r as w,s as T,t as E,u as D,v as O,w as k,x as A,y as j}from"./storybook-decorator-qg9t3HdX.js";var M,N,P,F,I,L,R=e((()=>{O(),t(),m(),M=n(),N=({workspaceName:e,planName:t,status:n,billingURL:m})=>(0,M.jsxs)(r,{lang:`en`,dir:`ltr`,children:[(0,M.jsx)(a,{}),(0,M.jsxs)(A,{children:[`Your subscription has been updated for `,e]}),(0,M.jsx)(v,{style:g,children:(0,M.jsxs)(i,{style:S,children:[(0,M.jsxs)(d,{style:D,children:[(0,M.jsx)(j,{style:b,children:`Bowrain`}),(0,M.jsx)(j,{style:l,children:`Localization platform`})]}),(0,M.jsxs)(d,{style:u,children:[(0,M.jsx)(c,{as:`h1`,style:P,children:`Your subscription has been updated`}),(0,M.jsxs)(j,{style:h,children:[`The subscription for `,(0,M.jsx)(`strong`,{children:e}),` has been updated. Here are the details:`]}),(0,M.jsxs)(d,{style:F,children:[(0,M.jsx)(j,{style:I,children:`Plan`}),(0,M.jsx)(j,{style:L,children:t}),(0,M.jsx)(j,{style:I,children:`Status`}),(0,M.jsx)(j,{style:L,children:n})]}),(0,M.jsx)(j,{style:h,children:`You can view your full billing details and manage your subscription from the billing page.`}),(0,M.jsx)(d,{style:_,children:(0,M.jsx)(o,{href:m,style:w,children:`View Billing`})}),(0,M.jsx)(k,{style:p}),(0,M.jsx)(j,{style:T,children:`Button not working? Copy and paste this link into your browser:`}),(0,M.jsx)(s,{href:m,style:C,children:m})]}),(0,M.jsxs)(d,{style:f,children:[(0,M.jsx)(j,{style:y,children:`© Bowrain. All rights reserved.`}),(0,M.jsx)(j,{style:y,children:`You received this email because you are an admin of this workspace.`})]})]})})]}),P={color:`#0f172a`,fontSize:`26px`,fontWeight:`700`,margin:`0 0 16px`,lineHeight:`1.2`},F={backgroundColor:`#f8fafc`,borderRadius:`8px`,border:`1px solid #e2e8f0`,padding:`20px 24px`,margin:`0 0 16px`},I={color:`#64748b`,fontSize:`13px`,fontWeight:`600`,margin:`0 0 2px`,textTransform:`uppercase`,letterSpacing:`0.5px`},L={color:`#0f172a`,fontSize:`16px`,fontWeight:`600`,margin:`0 0 12px`},N.__docgenInfo={description:`Branded subscription-changed email for Bowrain.

Props are populated at build time with Go text/template tokens
(e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML
doubles as a Go template. The mailer package fills in real values at
send time using text/template.Execute().`,methods:[],displayName:`SubscriptionChangedEmail`,props:{workspaceName:{required:!0,tsType:{name:`string`},description:``},planName:{required:!0,tsType:{name:`string`},description:``},status:{required:!0,tsType:{name:`string`},description:``},billingURL:{required:!0,tsType:{name:`string`},description:``}}}})),z,B,V,H,U,W,G,K;e((()=>{R(),x(),z=n(),B={title:`Emails/Subscription Changed`,component:N,tags:[`autodocs`],parameters:{layout:`padded`},decorators:[(e,{args:t})=>(0,z.jsx)(E,{children:(0,z.jsx)(N,{...t})})]},V={args:{workspaceName:`Acme Translations`,planName:`Pro`,status:`Active`,billingURL:`https://app.bowrain.com/acme/settings/billing`}},H={args:{workspaceName:`Globex Corp`,planName:`Team`,status:`Active`,billingURL:`https://app.bowrain.com/globex/settings/billing`}},U={args:{workspaceName:`Startup Inc`,planName:`Free`,status:`Active`,billingURL:`https://app.bowrain.com/startup/settings/billing`}},W={args:{workspaceName:`New Workspace`,planName:`Pro (Trial)`,status:`Trialing`,billingURL:`https://app.bowrain.com/new-workspace/settings/billing`}},G={args:{workspaceName:`Old Project`,planName:`Pro`,status:`Canceled`,billingURL:`https://app.bowrain.com/old-project/settings/billing`}},V.parameters={...V.parameters,docs:{...V.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Acme Translations",
    planName: "Pro",
    status: "Active",
    billingURL: "https://app.bowrain.com/acme/settings/billing"
  }
}`,...V.parameters?.docs?.source}}},H.parameters={...H.parameters,docs:{...H.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Globex Corp",
    planName: "Team",
    status: "Active",
    billingURL: "https://app.bowrain.com/globex/settings/billing"
  }
}`,...H.parameters?.docs?.source}}},U.parameters={...U.parameters,docs:{...U.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Startup Inc",
    planName: "Free",
    status: "Active",
    billingURL: "https://app.bowrain.com/startup/settings/billing"
  }
}`,...U.parameters?.docs?.source}}},W.parameters={...W.parameters,docs:{...W.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "New Workspace",
    planName: "Pro (Trial)",
    status: "Trialing",
    billingURL: "https://app.bowrain.com/new-workspace/settings/billing"
  }
}`,...W.parameters?.docs?.source}}},G.parameters={...G.parameters,docs:{...G.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Old Project",
    planName: "Pro",
    status: "Canceled",
    billingURL: "https://app.bowrain.com/old-project/settings/billing"
  }
}`,...G.parameters?.docs?.source}}},K=[`UpgradedToPro`,`UpgradedToTeam`,`DowngradedToFree`,`TrialStarted`,`Canceled`]}))();export{G as Canceled,U as DowngradedToFree,W as TrialStarted,V as UpgradedToPro,H as UpgradedToTeam,K as __namedExportsOrder,B as default};