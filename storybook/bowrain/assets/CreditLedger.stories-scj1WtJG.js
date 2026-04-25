import{n as e}from"./chunk-DnJy8xQt.js";import{S as t,x as n}from"./iframe-Cr-Gz2Kj.js";var r,i,a,o,s,c,l,u;e((()=>{t(),r={title:`Billing/CreditLedger`,component:n,tags:[`autodocs`]},i=Date.now(),a=3600*1e3,o=[{id:`1`,amount:5e5,balanceAfter:5e5,operation:`plan_reset`,createdAt:new Date(i-48*a).toISOString()},{id:`2`,amount:-12400,balanceAfter:487600,operation:`ai_translation`,referenceId:`job-abc12345`,createdAt:new Date(i-36*a).toISOString()},{id:`3`,amount:-3200,balanceAfter:484400,operation:`bravo_message`,referenceId:`conv-def67890`,createdAt:new Date(i-24*a).toISOString()},{id:`4`,amount:-800,balanceAfter:483600,operation:`bravo_container`,referenceId:`conv-def67890`,createdAt:new Date(i-23*a).toISOString()},{id:`5`,amount:-5100,balanceAfter:478500,operation:`ai_quality_check`,referenceId:`qa-ghi11223`,createdAt:new Date(i-12*a).toISOString()},{id:`6`,amount:2e5,balanceAfter:678500,operation:`purchase`,referenceId:`pi-jkl44556`,createdAt:new Date(i-6*a).toISOString()},{id:`7`,amount:5e4,balanceAfter:728500,operation:`grant`,createdAt:new Date(i-2*a).toISOString()},{id:`8`,amount:-18300,balanceAfter:710200,operation:`ai_translation`,referenceId:`job-mno77889`,createdAt:new Date(i-1*a).toISOString()}],s={args:{entries:o}},c={args:{entries:[]}},l={args:{entries:o.filter(e=>e.amount<0)}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    entries: sampleEntries
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    entries: []
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    entries: sampleEntries.filter(e => e.amount < 0)
  }
}`,...l.parameters?.docs?.source}}},u=[`Default`,`Empty`,`DebitsOnly`]}))();export{l as DebitsOnly,s as Default,c as Empty,u as __namedExportsOrder,r as default};