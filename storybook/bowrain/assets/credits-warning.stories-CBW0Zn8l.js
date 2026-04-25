import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./react-Baqbuk-D.js";import{t as n}from"./jsx-runtime-B-hFyic3.js";import{C as r,D as i,E as a,O as o,S as s,T as c,_ as l,a as u,b as d,c as f,d as p,f as m,g as h,h as g,i as _,k as v,l as y,m as b,n as x,o as S,p as C,r as w,s as T,t as E,u as D,v as O,w as k,x as A,y as j}from"./storybook-decorator-qg9t3HdX.js";var M,N,P,F,I,L=e((()=>{O(),t(),m(),M=n(),N=({workspaceName:e,usedCredits:t,totalCredits:n,usagePercent:m,resetDate:x,upgradeURL:E})=>(0,M.jsxs)(r,{lang:`en`,dir:`ltr`,children:[(0,M.jsx)(a,{}),(0,M.jsxs)(A,{children:[`Your AI credits are running low in `,e]}),(0,M.jsx)(v,{style:g,children:(0,M.jsxs)(i,{style:S,children:[(0,M.jsxs)(d,{style:D,children:[(0,M.jsx)(j,{style:b,children:`Bowrain`}),(0,M.jsx)(j,{style:l,children:`Localization platform`})]}),(0,M.jsxs)(d,{style:u,children:[(0,M.jsx)(c,{as:`h1`,style:P,children:`Your AI credits are running low`}),(0,M.jsxs)(j,{style:h,children:[`The workspace `,(0,M.jsx)(`strong`,{children:e}),` has used `,(0,M.jsxs)(`strong`,{children:[t,` of `,n]}),` AI credits (`,m,`%).`]}),(0,M.jsx)(d,{style:F,children:(0,M.jsx)(d,{style:I})}),(0,M.jsxs)(j,{style:h,children:[`Your credits will reset on `,(0,M.jsx)(`strong`,{children:x}),`. To avoid interruption, consider upgrading your plan for a higher credit allowance.`]}),(0,M.jsx)(d,{style:_,children:(0,M.jsx)(o,{href:E,style:w,children:`Upgrade Plan`})}),(0,M.jsx)(k,{style:p}),(0,M.jsx)(j,{style:T,children:`Button not working? Copy and paste this link into your browser:`}),(0,M.jsx)(s,{href:E,style:C,children:E})]}),(0,M.jsxs)(d,{style:f,children:[(0,M.jsx)(j,{style:y,children:`© Bowrain. All rights reserved.`}),(0,M.jsx)(j,{style:y,children:`You received this email because you are an admin of this workspace.`})]})]})})]}),P={color:`#0f172a`,fontSize:`26px`,fontWeight:`700`,margin:`0 0 16px`,lineHeight:`1.2`},F={backgroundColor:`#e2e8f0`,borderRadius:`6px`,height:`12px`,margin:`0 0 16px`,overflow:`hidden`},I={backgroundColor:`#f59e0b`,borderRadius:`6px`,height:`12px`,width:`80%`},N.__docgenInfo={description:`Branded credits-warning email for Bowrain.

Props are populated at build time with Go text/template tokens
(e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML
doubles as a Go template. The mailer package fills in real values at
send time using text/template.Execute().`,methods:[],displayName:`CreditsWarningEmail`,props:{workspaceName:{required:!0,tsType:{name:`string`},description:``},usedCredits:{required:!0,tsType:{name:`string`},description:``},totalCredits:{required:!0,tsType:{name:`string`},description:``},usagePercent:{required:!0,tsType:{name:`string`},description:``},resetDate:{required:!0,tsType:{name:`string`},description:``},upgradeURL:{required:!0,tsType:{name:`string`},description:``}}}})),R,z,B,V,H,U;e((()=>{L(),x(),R=n(),z={title:`Emails/Credits Warning`,component:N,tags:[`autodocs`],parameters:{layout:`padded`},decorators:[(e,{args:t})=>(0,R.jsx)(E,{children:(0,R.jsx)(N,{...t})})]},B={args:{workspaceName:`Acme Translations`,usedCredits:`400,000`,totalCredits:`500,000`,usagePercent:`80`,resetDate:`Monday, March 23, 2026`,upgradeURL:`https://app.bowrain.com/acme/settings/billing`}},V={args:{workspaceName:`Globex Corp`,usedCredits:`1,800,000`,totalCredits:`2,000,000`,usagePercent:`90`,resetDate:`Monday, March 30, 2026`,upgradeURL:`https://app.bowrain.com/globex/settings/billing`}},H={args:{workspaceName:`My Project`,usedCredits:`40,000`,totalCredits:`50,000`,usagePercent:`80`,resetDate:`Monday, March 23, 2026`,upgradeURL:`https://app.bowrain.com/my-project/settings/billing`}},B.parameters={...B.parameters,docs:{...B.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Acme Translations",
    usedCredits: "400,000",
    totalCredits: "500,000",
    usagePercent: "80",
    resetDate: "Monday, March 23, 2026",
    upgradeURL: "https://app.bowrain.com/acme/settings/billing"
  }
}`,...B.parameters?.docs?.source}}},V.parameters={...V.parameters,docs:{...V.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Globex Corp",
    usedCredits: "1,800,000",
    totalCredits: "2,000,000",
    usagePercent: "90",
    resetDate: "Monday, March 30, 2026",
    upgradeURL: "https://app.bowrain.com/globex/settings/billing"
  }
}`,...V.parameters?.docs?.source}}},H.parameters={...H.parameters,docs:{...H.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "My Project",
    usedCredits: "40,000",
    totalCredits: "50,000",
    usagePercent: "80",
    resetDate: "Monday, March 23, 2026",
    upgradeURL: "https://app.bowrain.com/my-project/settings/billing"
  }
}`,...H.parameters?.docs?.source}}},U=[`At80Percent`,`At90Percent`,`FreePlan`]}))();export{B as At80Percent,V as At90Percent,H as FreePlan,U as __namedExportsOrder,z as default};