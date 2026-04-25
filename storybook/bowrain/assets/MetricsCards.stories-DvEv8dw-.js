import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{N as n,P as r}from"./iframe-BJVhYAGS.js";function i(e){return new Intl.NumberFormat(`en-US`,{style:`currency`,currency:`USD`,minimumFractionDigits:0,maximumFractionDigits:0}).format(e)}function a(e){return`${e.toFixed(1)}%`}function o({metrics:e,loading:t}){return t||!e?(0,s.jsx)(`div`,{className:`grid grid-cols-3 gap-4`,children:Array.from({length:6}).map((e,t)=>(0,s.jsxs)(`div`,{className:`rounded-lg border p-4 space-y-2 animate-pulse`,children:[(0,s.jsx)(`div`,{className:`h-3 w-20 bg-muted rounded`}),(0,s.jsx)(`div`,{className:`h-6 w-16 bg-muted rounded`})]},t))}):(0,s.jsxs)(`div`,{className:`space-y-6`,children:[(0,s.jsxs)(`div`,{className:`grid grid-cols-3 gap-4`,children:[(0,s.jsxs)(`div`,{className:`rounded-lg border p-4 space-y-1`,children:[(0,s.jsx)(`p`,{className:`text-sm text-muted-foreground`,children:`MRR`}),(0,s.jsx)(`p`,{className:`text-2xl font-semibold`,children:i(e.mrr)})]}),(0,s.jsxs)(`div`,{className:`rounded-lg border p-4 space-y-1`,children:[(0,s.jsx)(`p`,{className:`text-sm text-muted-foreground`,children:`Active Workspaces`}),(0,s.jsx)(`p`,{className:`text-2xl font-semibold`,children:e.active_workspaces})]}),(0,s.jsxs)(`div`,{className:`rounded-lg border p-4 space-y-1`,children:[(0,s.jsx)(`p`,{className:`text-sm text-muted-foreground`,children:`New Signups`}),(0,s.jsx)(`p`,{className:`text-2xl font-semibold`,children:e.new_signups_7d}),(0,s.jsxs)(`p`,{className:`text-xs text-muted-foreground`,children:[e.new_signups_30d,` in last 30d`]})]}),(0,s.jsxs)(`div`,{className:`rounded-lg border p-4 space-y-1`,children:[(0,s.jsx)(`p`,{className:`text-sm text-muted-foreground`,children:`Credit Utilization`}),(0,s.jsx)(`p`,{className:`text-2xl font-semibold`,children:a(e.credit_utilization_percent)})]}),(0,s.jsxs)(`div`,{className:`rounded-lg border p-4 space-y-1`,children:[(0,s.jsx)(`p`,{className:`text-sm text-muted-foreground`,children:`Churn Rate`}),(0,s.jsx)(`p`,{className:`text-2xl font-semibold`,children:a(e.churn_rate_percent)})]})]}),(0,s.jsxs)(`div`,{className:`rounded-lg border`,children:[(0,s.jsx)(`div`,{className:`p-4 border-b`,children:(0,s.jsx)(`h3`,{className:`text-sm font-medium`,children:`Top 5 Workspaces by Usage`})}),(0,s.jsxs)(`table`,{className:`w-full text-sm`,children:[(0,s.jsx)(`thead`,{children:(0,s.jsxs)(`tr`,{className:`border-b bg-muted/50`,children:[(0,s.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Workspace`}),(0,s.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Plan`}),(0,s.jsx)(`th`,{className:`px-4 py-2 text-right font-medium`,children:`Credits Used`}),(0,s.jsx)(`th`,{className:`px-4 py-2 text-right font-medium`,children:`Credits Total`}),(0,s.jsx)(`th`,{className:`px-4 py-2 text-right font-medium`,children:`Usage`})]})}),(0,s.jsx)(`tbody`,{className:`divide-y`,children:e.top_workspaces.map(e=>(0,s.jsxs)(`tr`,{className:`hover:bg-muted/30`,children:[(0,s.jsx)(`td`,{className:`px-4 py-2`,children:e.workspace_name}),(0,s.jsx)(`td`,{className:`px-4 py-2`,children:(0,s.jsx)(n,{plan:e.plan,status:`active`})}),(0,s.jsx)(`td`,{className:`px-4 py-2 text-right tabular-nums`,children:e.credits_used.toLocaleString()}),(0,s.jsx)(`td`,{className:`px-4 py-2 text-right tabular-nums`,children:e.credits_total.toLocaleString()}),(0,s.jsx)(`td`,{className:`px-4 py-2 text-right tabular-nums`,children:e.credits_total>0?a(e.credits_used/e.credits_total*100):`0%`})]},e.workspace_id))})]})]})]})}var s,c,l,u,d,f,p,m;e((()=>{r(),s=t(),c={title:`Ctrl/MetricsCards`,component:o,tags:[`autodocs`]},l={mrr:12500,active_workspaces:150,new_signups_7d:23,new_signups_30d:87,credit_utilization_percent:65.5,churn_rate_percent:2.3,top_workspaces:[{workspace_id:`ws-1`,workspace_name:`Acme Corp`,plan:`team`,credits_used:18e5,credits_total:2e6},{workspace_id:`ws-2`,workspace_name:`Widget Inc`,plan:`pro`,credits_used:42e4,credits_total:5e5},{workspace_id:`ws-3`,workspace_name:`Startup Labs`,plan:`pro`,credits_used:35e4,credits_total:5e5},{workspace_id:`ws-4`,workspace_name:`Growth Co`,plan:`team`,credits_used:12e5,credits_total:2e6},{workspace_id:`ws-5`,workspace_name:`Indie Dev`,plan:`free`,credits_used:48e3,credits_total:5e4}]},u={args:{metrics:l,loading:!1}},d={args:{metrics:null,loading:!0}},f={args:{loading:!1,metrics:{...l,churn_rate_percent:8.5,credit_utilization_percent:92.1}}},p={args:{loading:!1,metrics:{mrr:500,active_workspaces:12,new_signups_7d:1,new_signups_30d:5,credit_utilization_percent:15.2,churn_rate_percent:0,top_workspaces:[{workspace_id:`ws-1`,workspace_name:`Solo Dev`,plan:`free`,credits_used:5e3,credits_total:5e4}]}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    metrics: sampleMetrics,
    loading: false
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    metrics: null,
    loading: true
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    loading: false,
    metrics: {
      ...sampleMetrics,
      churn_rate_percent: 8.5,
      credit_utilization_percent: 92.1
    }
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    loading: false,
    metrics: {
      mrr: 500,
      active_workspaces: 12,
      new_signups_7d: 1,
      new_signups_30d: 5,
      credit_utilization_percent: 15.2,
      churn_rate_percent: 0,
      top_workspaces: [{
        workspace_id: "ws-1",
        workspace_name: "Solo Dev",
        plan: "free",
        credits_used: 5000,
        credits_total: 50000
      }]
    }
  }
}`,...p.parameters?.docs?.source}}},m=[`Default`,`Loading`,`HighChurn`,`LowActivity`]}))();export{u as Default,f as HighChurn,d as Loading,p as LowActivity,m as __namedExportsOrder,c as default};