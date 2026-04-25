import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{t as n}from"./src-Bvgh6dl3.js";import{t as r}from"./badge-CyOz_y6s.js";import{N as i,P as a}from"./iframe-Cr-Gz2Kj.js";function o({upsells:e,loading:t,onRowClick:n}){return t?(0,s.jsx)(`div`,{className:`rounded-lg border p-8 text-center text-sm text-muted-foreground`,children:`Loading upsell opportunities...`}):e.length===0?(0,s.jsx)(`div`,{className:`rounded-lg border p-8 text-center text-sm text-muted-foreground`,children:`No upsell opportunities detected.`}):(0,s.jsx)(`div`,{className:`rounded-lg border`,children:(0,s.jsxs)(`table`,{className:`w-full text-sm`,children:[(0,s.jsx)(`thead`,{children:(0,s.jsxs)(`tr`,{className:`border-b bg-muted/50`,children:[(0,s.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Workspace`}),(0,s.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Signal`}),(0,s.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Current Plan`}),(0,s.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Suggested`}),(0,s.jsx)(`th`,{className:`px-4 py-2 text-right font-medium`,children:`Score`}),(0,s.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Detail`}),(0,s.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Detected`})]})}),(0,s.jsx)(`tbody`,{className:`divide-y`,children:e.map(e=>{let t=c[e.signal];return(0,s.jsxs)(`tr`,{onClick:()=>n(e.workspace_id),className:`hover:bg-muted/30 cursor-pointer`,children:[(0,s.jsx)(`td`,{className:`px-4 py-2 font-medium`,children:e.workspace_name}),(0,s.jsx)(`td`,{className:`px-4 py-2`,children:(0,s.jsx)(`span`,{className:`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${t.className}`,children:t.label})}),(0,s.jsx)(`td`,{className:`px-4 py-2`,children:(0,s.jsx)(i,{plan:e.current_plan,status:`active`})}),(0,s.jsx)(`td`,{className:`px-4 py-2`,children:(0,s.jsx)(r,{variant:`outline`,children:e.suggested_plan})}),(0,s.jsx)(`td`,{className:`px-4 py-2 text-right tabular-nums font-medium`,children:e.score}),(0,s.jsx)(`td`,{className:`px-4 py-2 text-muted-foreground max-w-xs truncate`,children:e.detail}),(0,s.jsx)(`td`,{className:`px-4 py-2 text-muted-foreground`,children:new Date(e.detected_at).toLocaleDateString()})]},`${e.workspace_id}-${e.signal}`)})})]})})}var s,c,l,u,d,f,p,m,h;e((()=>{n(),a(),s=t(),c={credit_exhaustion:{label:`Credit Exhaustion`,className:`bg-destructive/10 text-destructive dark:bg-destructive/20 dark:text-destructive`},seat_pressure:{label:`Seat Pressure`,className:`bg-warning/10 text-warning dark:bg-warning/20 dark:text-warning`},feature_gate_hits:{label:`Feature Gate Hits`,className:`bg-info/10 text-info dark:bg-info/20 dark:text-info`},high_usage:{label:`High Usage`,className:`bg-warning/10 text-warning dark:bg-warning/20 dark:text-warning`},trial_expiring:{label:`Trial Expiring`,className:`bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400`},dormant_paid:{label:`Dormant Paid`,className:`bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400`}},l={title:`Ctrl/UpsellTable`,component:o,tags:[`autodocs`]},u=[{workspace_id:`ws-1`,workspace_name:`Acme Corp`,current_plan:`free`,signal:`credit_exhaustion`,score:90,detail:`Exhausted credits 3 weeks in a row`,suggested_plan:`pro`,detected_at:`2026-03-18T00:00:00Z`},{workspace_id:`ws-2`,workspace_name:`Widget Inc`,current_plan:`pro`,signal:`high_usage`,score:75,detail:`Average credit usage 92% over 3 weeks`,suggested_plan:`team`,detected_at:`2026-03-17T00:00:00Z`},{workspace_id:`ws-3`,workspace_name:`Growth Co`,current_plan:`pro`,signal:`seat_pressure`,score:70,detail:`3/3 seats used, invited 2 more`,suggested_plan:`team`,detected_at:`2026-03-16T00:00:00Z`},{workspace_id:`ws-4`,workspace_name:`Startup Labs`,current_plan:`free`,signal:`feature_gate_hits`,score:80,detail:`Hit feature gates 12 times this week`,suggested_plan:`pro`,detected_at:`2026-03-15T00:00:00Z`},{workspace_id:`ws-5`,workspace_name:`Old Corp`,current_plan:`team`,signal:`dormant_paid`,score:50,detail:`Average credit usage 3% over 4 weeks`,suggested_plan:`team`,detected_at:`2026-03-14T00:00:00Z`}],d={args:{upsells:u,loading:!1,onRowClick:()=>{}}},f={args:{upsells:[],loading:!0,onRowClick:()=>{}}},p={args:{upsells:[],loading:!1,onRowClick:()=>{}}},m={args:{loading:!1,onRowClick:()=>{},upsells:[{workspace_id:`ws-1`,workspace_name:`WS-1`,current_plan:`free`,signal:`credit_exhaustion`,score:90,detail:`Credit exhaustion detail`,suggested_plan:`pro`,detected_at:`2026-03-20T00:00:00Z`},{workspace_id:`ws-2`,workspace_name:`WS-2`,current_plan:`pro`,signal:`seat_pressure`,score:75,detail:`Seat pressure detail`,suggested_plan:`team`,detected_at:`2026-03-20T00:00:00Z`},{workspace_id:`ws-3`,workspace_name:`WS-3`,current_plan:`free`,signal:`feature_gate_hits`,score:80,detail:`Feature gate detail`,suggested_plan:`pro`,detected_at:`2026-03-20T00:00:00Z`},{workspace_id:`ws-4`,workspace_name:`WS-4`,current_plan:`pro`,signal:`high_usage`,score:70,detail:`High usage detail`,suggested_plan:`team`,detected_at:`2026-03-20T00:00:00Z`},{workspace_id:`ws-5`,workspace_name:`WS-5`,current_plan:`pro`,signal:`trial_expiring`,score:85,detail:`Trial expiring detail`,suggested_plan:`pro`,detected_at:`2026-03-20T00:00:00Z`},{workspace_id:`ws-6`,workspace_name:`WS-6`,current_plan:`team`,signal:`dormant_paid`,score:50,detail:`Dormant paid detail`,suggested_plan:`team`,detected_at:`2026-03-20T00:00:00Z`}]}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    upsells: sampleUpsells,
    loading: false,
    onRowClick: () => {}
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    upsells: [],
    loading: true,
    onRowClick: () => {}
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    upsells: [],
    loading: false,
    onRowClick: () => {}
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    loading: false,
    onRowClick: () => {},
    upsells: [{
      workspace_id: "ws-1",
      workspace_name: "WS-1",
      current_plan: "free",
      signal: "credit_exhaustion",
      score: 90,
      detail: "Credit exhaustion detail",
      suggested_plan: "pro",
      detected_at: "2026-03-20T00:00:00Z"
    }, {
      workspace_id: "ws-2",
      workspace_name: "WS-2",
      current_plan: "pro",
      signal: "seat_pressure",
      score: 75,
      detail: "Seat pressure detail",
      suggested_plan: "team",
      detected_at: "2026-03-20T00:00:00Z"
    }, {
      workspace_id: "ws-3",
      workspace_name: "WS-3",
      current_plan: "free",
      signal: "feature_gate_hits",
      score: 80,
      detail: "Feature gate detail",
      suggested_plan: "pro",
      detected_at: "2026-03-20T00:00:00Z"
    }, {
      workspace_id: "ws-4",
      workspace_name: "WS-4",
      current_plan: "pro",
      signal: "high_usage",
      score: 70,
      detail: "High usage detail",
      suggested_plan: "team",
      detected_at: "2026-03-20T00:00:00Z"
    }, {
      workspace_id: "ws-5",
      workspace_name: "WS-5",
      current_plan: "pro",
      signal: "trial_expiring",
      score: 85,
      detail: "Trial expiring detail",
      suggested_plan: "pro",
      detected_at: "2026-03-20T00:00:00Z"
    }, {
      workspace_id: "ws-6",
      workspace_name: "WS-6",
      current_plan: "team",
      signal: "dormant_paid",
      score: 50,
      detail: "Dormant paid detail",
      suggested_plan: "team",
      detected_at: "2026-03-20T00:00:00Z"
    }]
  }
}`,...m.parameters?.docs?.source}}},h=[`Default`,`Loading`,`Empty`,`AllSignalTypes`]}))();export{m as AllSignalTypes,d as Default,p as Empty,f as Loading,h as __namedExportsOrder,l as default};