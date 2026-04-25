import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{N as n,P as r}from"./iframe-Cr-Gz2Kj.js";function i({workspaces:e,loading:t,onRowClick:r}){return t?(0,a.jsx)(`div`,{className:`rounded-lg border p-8 text-center text-sm text-muted-foreground`,children:`Loading workspaces...`}):e.length===0?(0,a.jsx)(`div`,{className:`rounded-lg border p-8 text-center text-sm text-muted-foreground`,children:`No workspaces found.`}):(0,a.jsx)(`div`,{className:`rounded-lg border`,children:(0,a.jsxs)(`table`,{className:`w-full text-sm`,children:[(0,a.jsx)(`thead`,{children:(0,a.jsxs)(`tr`,{className:`border-b bg-muted/50`,children:[(0,a.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Name`}),(0,a.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Owner`}),(0,a.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Plan`}),(0,a.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Status`}),(0,a.jsx)(`th`,{className:`px-4 py-2 text-right font-medium`,children:`Members`}),(0,a.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Created`})]})}),(0,a.jsx)(`tbody`,{className:`divide-y`,children:e.map(e=>(0,a.jsxs)(`tr`,{onClick:()=>r(e.id),className:`hover:bg-muted/30 cursor-pointer`,children:[(0,a.jsx)(`td`,{className:`px-4 py-2 font-medium`,children:e.name}),(0,a.jsx)(`td`,{className:`px-4 py-2 text-muted-foreground`,children:e.owner_email}),(0,a.jsx)(`td`,{className:`px-4 py-2`,children:(0,a.jsx)(n,{plan:e.plan,status:e.status})}),(0,a.jsx)(`td`,{className:`px-4 py-2 text-muted-foreground`,children:e.status}),(0,a.jsx)(`td`,{className:`px-4 py-2 text-right tabular-nums`,children:e.member_count}),(0,a.jsx)(`td`,{className:`px-4 py-2 text-muted-foreground`,children:new Date(e.created_at).toLocaleDateString()})]},e.id))})]})})}var a,o,s,c,l,u,d,f;e((()=>{r(),a=t(),o={title:`Ctrl/WorkspaceTable`,component:i,tags:[`autodocs`]},s=[{id:`ws-1`,name:`Acme Corp`,owner_email:`admin@acme.com`,plan:`team`,status:`active`,credits_used:15e5,credits_total:2e6,member_count:12,created_at:`2025-06-15T00:00:00Z`},{id:`ws-2`,name:`Widget Inc`,owner_email:`ceo@widget.io`,plan:`pro`,status:`active`,credits_used:3e5,credits_total:5e5,member_count:3,created_at:`2025-09-01T00:00:00Z`},{id:`ws-3`,name:`Startup Labs`,owner_email:`dev@startup.co`,plan:`free`,status:`active`,credits_used:48e3,credits_total:5e4,member_count:1,created_at:`2026-01-10T00:00:00Z`},{id:`ws-4`,name:`Old Corp`,owner_email:`legacy@old.com`,plan:`pro`,status:`canceled`,credits_used:0,credits_total:5e5,member_count:2,created_at:`2024-11-20T00:00:00Z`},{id:`ws-5`,name:`Enterprise Co`,owner_email:`ops@enterprise.com`,plan:`enterprise`,status:`active`,credits_used:5e6,credits_total:1e7,member_count:50,created_at:`2025-01-01T00:00:00Z`}],c={args:{workspaces:s,loading:!1,onRowClick:()=>{}}},l={args:{workspaces:[],loading:!0,onRowClick:()=>{}}},u={args:{workspaces:[],loading:!1,onRowClick:()=>{}}},d={args:{workspaces:[s[0]],loading:!1,onRowClick:()=>{}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    workspaces: sampleWorkspaces,
    loading: false,
    onRowClick: () => {}
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    workspaces: [],
    loading: true,
    onRowClick: () => {}
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    workspaces: [],
    loading: false,
    onRowClick: () => {}
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    workspaces: [sampleWorkspaces[0]],
    loading: false,
    onRowClick: () => {}
  }
}`,...d.parameters?.docs?.source}}},f=[`Default`,`Loading`,`Empty`,`SingleWorkspace`]}))();export{c as Default,u as Empty,l as Loading,d as SingleWorkspace,f as __namedExportsOrder,o as default};