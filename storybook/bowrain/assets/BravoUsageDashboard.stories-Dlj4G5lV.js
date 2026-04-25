import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{F as n,I as r}from"./iframe-Cr-Gz2Kj.js";import{a as i,t as a}from"./fixtures-DC2kBE5i.js";var o,s,c,l,u,d,f,p;e((()=>{r(),a(),o=t(),{fn:s}=__STORYBOOK_MODULE_TEST__,c={title:`Bravo/BravoUsageDashboard`,component:n,tags:[`autodocs`],decorators:[e=>(0,o.jsx)(`div`,{style:{maxWidth:600,padding:16},children:(0,o.jsx)(e,{})})]},l={args:{usage:i}},u={args:{usage:i,onDateRangeChange:s()}},d={args:{usage:{workspace_id:`ws-1`,total_input_tokens:1200,total_output_tokens:400,total_container_sec:45,message_count:3}}},f={args:{usage:{workspace_id:`ws-1`,total_input_tokens:245e4,total_output_tokens:89e4,total_container_sec:48200,message_count:1240},onDateRangeChange:s()}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    usage: sampleUsage
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    usage: sampleUsage,
    onDateRangeChange: fn()
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    usage: {
      workspace_id: "ws-1",
      total_input_tokens: 1200,
      total_output_tokens: 400,
      total_container_sec: 45,
      message_count: 3
    }
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    usage: {
      workspace_id: "ws-1",
      total_input_tokens: 2450000,
      total_output_tokens: 890000,
      total_container_sec: 48200,
      message_count: 1240
    },
    onDateRangeChange: fn()
  }
}`,...f.parameters?.docs?.source}}},p=[`Default`,`WithDateRange`,`LowUsage`,`HighUsage`]}))();export{l as Default,f as HighUsage,d as LowUsage,u as WithDateRange,p as __namedExportsOrder,c as default};