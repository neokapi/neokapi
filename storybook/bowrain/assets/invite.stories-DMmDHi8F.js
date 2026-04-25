import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./react-Baqbuk-D.js";import{t as n}from"./jsx-runtime-B-hFyic3.js";import{C as r,D as i,E as a,O as o,S as s,T as c,_ as l,a as u,b as d,c as f,d as p,f as m,g as h,h as g,i as _,k as v,l as y,m as b,n as x,o as S,p as C,r as w,s as T,t as E,u as D,v as O,w as k,x as A,y as j}from"./storybook-decorator-qg9t3HdX.js";var M,N,P,F=e((()=>{O(),t(),m(),M=n(),N=({workspaceName:e,role:t,joinURL:n})=>(0,M.jsxs)(r,{lang:`en`,dir:`ltr`,children:[(0,M.jsx)(a,{}),(0,M.jsxs)(A,{children:[`You've been invited to join `,e,` on Bowrain`]}),(0,M.jsx)(v,{style:g,children:(0,M.jsxs)(i,{style:S,children:[(0,M.jsxs)(d,{style:D,children:[(0,M.jsx)(j,{style:b,children:`Bowrain`}),(0,M.jsx)(j,{style:l,children:`Localization platform`})]}),(0,M.jsxs)(d,{style:u,children:[(0,M.jsx)(c,{as:`h1`,style:P,children:`You're Invited 🎉`}),(0,M.jsxs)(j,{style:h,children:[`You've been invited to join `,(0,M.jsx)(`strong`,{children:e}),` on Bowrain as `,(0,M.jsx)(`strong`,{children:t}),`.`]}),(0,M.jsx)(j,{style:h,children:`Click the button below to accept the invitation and get started.`}),(0,M.jsx)(d,{style:_,children:(0,M.jsx)(o,{href:n,style:w,children:`Accept Invitation`})}),(0,M.jsx)(k,{style:p}),(0,M.jsx)(j,{style:T,children:`Button not working? Copy and paste this link into your browser:`}),(0,M.jsx)(s,{href:n,style:C,children:n})]}),(0,M.jsxs)(d,{style:f,children:[(0,M.jsx)(j,{style:y,children:`© Bowrain. All rights reserved.`}),(0,M.jsx)(j,{style:y,children:`If you didn't request this invitation, you can safely ignore this email.`})]})]})})]}),P={color:`#0f172a`,fontSize:`26px`,fontWeight:`700`,margin:`0 0 16px`,lineHeight:`1.2`},N.__docgenInfo={description:`Branded invitation email for Bowrain.

Props are populated at build time with Go text/template tokens
(e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML
doubles as a Go template. The mailer package fills in real values at
send time using text/template.Execute().`,methods:[],displayName:`InviteEmail`,props:{workspaceName:{required:!0,tsType:{name:`string`},description:``},role:{required:!0,tsType:{name:`string`},description:``},joinURL:{required:!0,tsType:{name:`string`},description:``}}}})),I,L,R,z,B,V;e((()=>{F(),x(),I=n(),L={title:`Emails/Invite`,component:N,tags:[`autodocs`],parameters:{layout:`padded`},decorators:[(e,{args:t})=>(0,I.jsx)(E,{children:(0,I.jsx)(N,{...t})})]},R={args:{workspaceName:`Acme Translations`,role:`editor`,joinURL:`https://app.bowrain.com/invite/abc123`}},z={args:{workspaceName:`Globex Corp`,role:`owner`,joinURL:`https://app.bowrain.com/invite/xyz789`}},B={args:{workspaceName:`Startup Inc`,role:`viewer`,joinURL:`https://app.bowrain.com/invite/viewer-456`}},R.parameters={...R.parameters,docs:{...R.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Acme Translations",
    role: "editor",
    joinURL: "https://app.bowrain.com/invite/abc123"
  }
}`,...R.parameters?.docs?.source}}},z.parameters={...z.parameters,docs:{...z.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Globex Corp",
    role: "owner",
    joinURL: "https://app.bowrain.com/invite/xyz789"
  }
}`,...z.parameters?.docs?.source}}},B.parameters={...B.parameters,docs:{...B.parameters?.docs,source:{originalSource:`{
  args: {
    workspaceName: "Startup Inc",
    role: "viewer",
    joinURL: "https://app.bowrain.com/invite/viewer-456"
  }
}`,...B.parameters?.docs?.source}}},V=[`Default`,`OwnerInvite`,`ViewerInvite`]}))();export{R as Default,z as OwnerInvite,B as ViewerInvite,V as __namedExportsOrder,L as default};