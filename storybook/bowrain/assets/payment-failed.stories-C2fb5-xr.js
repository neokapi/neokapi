import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./react-Baqbuk-D.js";import{t as n}from"./jsx-runtime-B-hFyic3.js";import{C as r,D as i,E as a,O as o,S as s,T as c,_ as l,a as u,b as d,c as f,d as p,f as m,g as h,h as g,i as _,k as v,l as y,m as b,n as x,o as S,p as C,r as w,s as T,t as E,u as D,v as O,w as k,x as A,y as j}from"./storybook-decorator-qg9t3HdX.js";var M,N,P,F=e((()=>{O(),t(),m(),M=n(),N=({workspaceName:e,invoiceAmount:t,currency:n,updatePaymentURL:m})=>(0,M.jsxs)(r,{lang:`en`,dir:`ltr`,children:[(0,M.jsx)(a,{}),(0,M.jsxs)(A,{children:[`Payment failed for `,e]}),(0,M.jsx)(v,{style:g,children:(0,M.jsxs)(i,{style:S,children:[(0,M.jsxs)(d,{style:D,children:[(0,M.jsx)(j,{style:b,children:`Bowrain`}),(0,M.jsx)(j,{style:l,children:`Localization platform`})]}),(0,M.jsxs)(d,{style:u,children:[(0,M.jsx)(c,{as:`h1`,style:P,children:`Payment failed for your subscription`}),(0,M.jsxs)(j,{style:h,children:[`We were unable to process the payment of `,(0,M.jsxs)(`strong`,{children:[t,` `,n]}),` for the workspace `,(0,M.jsx)(`strong`,{children:e}),`.`]}),(0,M.jsxs)(j,{style:h,children:[`Your subscription is still active, but you have a `,(0,M.jsx)(`strong`,{children:`7-day grace period`}),` to update your payment method. If the payment is not resolved within this period, your subscription will be downgraded to the free plan.`]}),(0,M.jsx)(d,{style:_,children:(0,M.jsx)(o,{href:m,style:w,children:`Update Payment Method`})}),(0,M.jsx)(k,{style:p}),(0,M.jsx)(j,{style:T,children:`Button not working? Copy and paste this link into your browser:`}),(0,M.jsx)(s,{href:m,style:C,children:m})]}),(0,M.jsxs)(d,{style:f,children:[(0,M.jsx)(j,{style:y,children:`© Bowrain. All rights reserved.`}),(0,M.jsx)(j,{style:y,children:`You received this email because you are an admin of this workspace.`})]})]})})]}),P={color:`#0f172a`,fontSize:`26px`,fontWeight:`700`,margin:`0 0 16px`,lineHeight:`1.2`},N.__docgenInfo={description:`Branded payment-failed email for Bowrain.

Props are populated at build time with Go text/template tokens
(e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML
doubles as a Go template. The mailer package fills in real values at
send time using text/template.Execute().`,methods:[],displayName:`PaymentFailedEmail`,props:{workspaceName:{required:!0,tsType:{name:`string`},description:``},invoiceAmount:{required:!0,tsType:{name:`string`},description:``},currency:{required:!0,tsType:{name:`string`},description:``},updatePaymentURL:{required:!0,tsType:{name:`string`},description:``}}}})),I,L,R,z,B,V;e((()=>{F(),x(),I=n(),L={title:`Emails/Payment Failed`,component:N,tags:[`autodocs`],parameters:{layout:`padded`},decorators:[(e,{args:t})=>(0,I.jsx)(E,{children:(0,I.jsx)(N,{...t})})]},R={args:{workspaceName:`Acme Translations`,invoiceAmount:`$25.00`,currency:`USD`,updatePaymentURL:`https://app.bowrain.com/acme/settings/billing`}},z={args:{workspaceName:`Globex Engineering`,invoiceAmount:`$100.00`,currency:`USD`,updatePaymentURL:`https://app.bowrain.com/globex/settings/billing`}},B={args:{workspaceName:`Berlin Localization GmbH`,invoiceAmount:`€45.00`,currency:`EUR`,updatePaymentURL:`https://app.bowrain.com/berlin-loc/settings/billing`}},R.parameters={...R.parameters,docs:{...R.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Acme Translations",
    invoiceAmount: "$25.00",
    currency: "USD",
    updatePaymentURL: "https://app.bowrain.com/acme/settings/billing"
  }
}`,...R.parameters?.docs?.source}}},z.parameters={...z.parameters,docs:{...z.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Globex Engineering",
    invoiceAmount: "$100.00",
    currency: "USD",
    updatePaymentURL: "https://app.bowrain.com/globex/settings/billing"
  }
}`,...z.parameters?.docs?.source}}},B.parameters={...B.parameters,docs:{...B.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Berlin Localization GmbH",
    invoiceAmount: "€45.00",
    currency: "EUR",
    updatePaymentURL: "https://app.bowrain.com/berlin-loc/settings/billing"
  }
}`,...B.parameters?.docs?.source}}},V=[`MonthlySubscription`,`TeamPlan`,`EuroCurrency`]}))();export{B as EuroCurrency,R as MonthlySubscription,z as TeamPlan,V as __namedExportsOrder,L as default};