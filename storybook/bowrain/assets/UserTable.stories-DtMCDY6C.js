import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{t as n}from"./utils-BUNg-TRL.js";import{t as r}from"./src-Q72DuJW2.js";function i({users:e,loading:t,selectedUserId:r,onRowClick:i}){return t?(0,a.jsx)(`div`,{className:`rounded-lg border p-8 text-center text-sm text-muted-foreground`,children:`Loading users...`}):e.length===0?(0,a.jsx)(`div`,{className:`rounded-lg border p-8 text-center text-sm text-muted-foreground`,children:`No users found.`}):(0,a.jsx)(`div`,{className:`rounded-lg border`,children:(0,a.jsxs)(`table`,{className:`w-full text-sm`,children:[(0,a.jsx)(`thead`,{children:(0,a.jsxs)(`tr`,{className:`border-b bg-muted/50`,children:[(0,a.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Email`}),(0,a.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Name`}),(0,a.jsx)(`th`,{className:`px-4 py-2 text-right font-medium`,children:`Workspaces`}),(0,a.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Last Login`}),(0,a.jsx)(`th`,{className:`px-4 py-2 text-left font-medium`,children:`Created`})]})}),(0,a.jsx)(`tbody`,{className:`divide-y`,children:e.map(e=>(0,a.jsxs)(`tr`,{onClick:()=>i(e.id),className:n(`hover:bg-muted/30 cursor-pointer`,r===e.id&&`bg-muted/50`),children:[(0,a.jsx)(`td`,{className:`px-4 py-2 font-medium`,children:e.email}),(0,a.jsx)(`td`,{className:`px-4 py-2`,children:e.name}),(0,a.jsx)(`td`,{className:`px-4 py-2 text-right tabular-nums`,children:e.workspace_count}),(0,a.jsx)(`td`,{className:`px-4 py-2 text-muted-foreground`,children:e.last_login?new Date(e.last_login).toLocaleDateString():`Never`}),(0,a.jsx)(`td`,{className:`px-4 py-2 text-muted-foreground`,children:new Date(e.created_at).toLocaleDateString()})]},e.id))})]})})}var a,o,s,c,l,u,d,f;e((()=>{r(),a=t(),o={title:`Ctrl/UserTable`,component:i,tags:[`autodocs`]},s=[{id:`u-1`,email:`admin@acme.com`,name:`Alice Admin`,workspace_count:3,last_login:`2026-03-19T10:30:00Z`,created_at:`2025-01-15T00:00:00Z`},{id:`u-2`,email:`bob@widget.io`,name:`Bob Builder`,workspace_count:1,last_login:`2026-03-18T14:00:00Z`,created_at:`2025-06-01T00:00:00Z`},{id:`u-3`,email:`charlie@startup.co`,name:`Charlie Dev`,workspace_count:2,last_login:null,created_at:`2026-02-20T00:00:00Z`},{id:`u-4`,email:`diana@enterprise.com`,name:`Diana Ops`,workspace_count:5,last_login:`2026-03-20T08:00:00Z`,created_at:`2024-11-01T00:00:00Z`}],c={args:{users:s,loading:!1,selectedUserId:null,onRowClick:()=>{}}},l={args:{users:[],loading:!0,selectedUserId:null,onRowClick:()=>{}}},u={args:{users:[],loading:!1,selectedUserId:null,onRowClick:()=>{}}},d={args:{users:s,loading:!1,selectedUserId:`u-2`,onRowClick:()=>{}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    users: sampleUsers,
    loading: false,
    selectedUserId: null,
    onRowClick: () => {}
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    users: [],
    loading: true,
    selectedUserId: null,
    onRowClick: () => {}
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    users: [],
    loading: false,
    selectedUserId: null,
    onRowClick: () => {}
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    users: sampleUsers,
    loading: false,
    selectedUserId: "u-2",
    onRowClick: () => {}
  }
}`,...d.parameters?.docs?.source}}},f=[`Default`,`Loading`,`Empty`,`WithSelection`]}))();export{c as Default,u as Empty,l as Loading,d as WithSelection,f as __namedExportsOrder,o as default};