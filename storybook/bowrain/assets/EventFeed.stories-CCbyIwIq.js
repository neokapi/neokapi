import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{t as n}from"./src-Q72DuJW2.js";import{t as r}from"./badge-CyOz_y6s.js";function i({events:e,loading:t}){return t?(0,a.jsx)(`div`,{className:`rounded-lg border p-8 text-center text-sm text-muted-foreground`,children:`Loading events...`}):e.length===0?(0,a.jsx)(`div`,{className:`rounded-lg border p-8 text-center text-sm text-muted-foreground`,children:`No billing events found.`}):(0,a.jsx)(`div`,{className:`space-y-2`,children:e.map(e=>(0,a.jsxs)(`div`,{className:`flex items-center justify-between rounded-lg border px-4 py-3`,children:[(0,a.jsxs)(`div`,{className:`flex items-center gap-3`,children:[(0,a.jsx)(r,{variant:o[e.type],children:s[e.type]}),(0,a.jsxs)(`div`,{children:[(0,a.jsx)(`p`,{className:`text-sm font-medium`,children:e.workspace_name}),(0,a.jsx)(`p`,{className:`text-xs text-muted-foreground`,children:e.detail})]})]}),(0,a.jsx)(`span`,{className:`text-xs text-muted-foreground whitespace-nowrap`,children:new Date(e.created_at).toLocaleString()})]},e.id))})}var a,o,s,c,l,u,d,f,p,m;e((()=>{n(),a=t(),o={subscription_created:`default`,subscription_upgraded:`default`,subscription_downgraded:`secondary`,subscription_canceled:`destructive`,payment_succeeded:`default`,payment_failed:`destructive`,credits_purchased:`outline`,credits_granted:`outline`},s={subscription_created:`Created`,subscription_upgraded:`Upgraded`,subscription_downgraded:`Downgraded`,subscription_canceled:`Canceled`,payment_succeeded:`Payment OK`,payment_failed:`Payment Failed`,credits_purchased:`Credits Purchased`,credits_granted:`Credits Granted`},c={title:`Ctrl/EventFeed`,component:i,tags:[`autodocs`]},l=[{id:`1`,type:`subscription_created`,workspace_name:`Acme Corp`,detail:`Pro plan activated`,created_at:`2026-03-18T14:30:00Z`},{id:`2`,type:`payment_failed`,workspace_name:`Widget Inc`,detail:`Invoice inv_123, amount=$25 USD`,created_at:`2026-03-17T09:15:00Z`},{id:`3`,type:`credits_granted`,workspace_name:`Startup Labs`,detail:`Granted 100K credits: support compensation`,created_at:`2026-03-16T11:00:00Z`},{id:`4`,type:`subscription_canceled`,workspace_name:`Old Corp`,detail:`Downgraded to free`,created_at:`2026-03-15T16:45:00Z`},{id:`5`,type:`subscription_upgraded`,workspace_name:`Growth Inc`,detail:`Upgraded from Pro to Team`,created_at:`2026-03-14T08:20:00Z`}],u={args:{events:l,loading:!1}},d={args:{events:[],loading:!0}},f={args:{events:[],loading:!1}},p={args:{loading:!1,events:[{id:`1`,type:`subscription_created`,workspace_name:`WS-1`,detail:`Created`,created_at:`2026-03-20T00:00:00Z`},{id:`2`,type:`subscription_upgraded`,workspace_name:`WS-2`,detail:`Upgraded`,created_at:`2026-03-20T00:00:00Z`},{id:`3`,type:`subscription_downgraded`,workspace_name:`WS-3`,detail:`Downgraded`,created_at:`2026-03-20T00:00:00Z`},{id:`4`,type:`subscription_canceled`,workspace_name:`WS-4`,detail:`Canceled`,created_at:`2026-03-20T00:00:00Z`},{id:`5`,type:`payment_succeeded`,workspace_name:`WS-5`,detail:`Payment OK`,created_at:`2026-03-20T00:00:00Z`},{id:`6`,type:`payment_failed`,workspace_name:`WS-6`,detail:`Payment Failed`,created_at:`2026-03-20T00:00:00Z`},{id:`7`,type:`credits_purchased`,workspace_name:`WS-7`,detail:`Purchased`,created_at:`2026-03-20T00:00:00Z`},{id:`8`,type:`credits_granted`,workspace_name:`WS-8`,detail:`Granted`,created_at:`2026-03-20T00:00:00Z`}]}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    events: sampleEvents,
    loading: false
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    events: [],
    loading: true
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    events: [],
    loading: false
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    loading: false,
    events: [{
      id: "1",
      type: "subscription_created",
      workspace_name: "WS-1",
      detail: "Created",
      created_at: "2026-03-20T00:00:00Z"
    }, {
      id: "2",
      type: "subscription_upgraded",
      workspace_name: "WS-2",
      detail: "Upgraded",
      created_at: "2026-03-20T00:00:00Z"
    }, {
      id: "3",
      type: "subscription_downgraded",
      workspace_name: "WS-3",
      detail: "Downgraded",
      created_at: "2026-03-20T00:00:00Z"
    }, {
      id: "4",
      type: "subscription_canceled",
      workspace_name: "WS-4",
      detail: "Canceled",
      created_at: "2026-03-20T00:00:00Z"
    }, {
      id: "5",
      type: "payment_succeeded",
      workspace_name: "WS-5",
      detail: "Payment OK",
      created_at: "2026-03-20T00:00:00Z"
    }, {
      id: "6",
      type: "payment_failed",
      workspace_name: "WS-6",
      detail: "Payment Failed",
      created_at: "2026-03-20T00:00:00Z"
    }, {
      id: "7",
      type: "credits_purchased",
      workspace_name: "WS-7",
      detail: "Purchased",
      created_at: "2026-03-20T00:00:00Z"
    }, {
      id: "8",
      type: "credits_granted",
      workspace_name: "WS-8",
      detail: "Granted",
      created_at: "2026-03-20T00:00:00Z"
    }]
  }
}`,...p.parameters?.docs?.source}}},m=[`Default`,`Loading`,`Empty`,`AllEventTypes`]}))();export{p as AllEventTypes,u as Default,f as Empty,d as Loading,m as __namedExportsOrder,c as default};