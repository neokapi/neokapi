import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{t as i}from"./utils-BUNg-TRL.js";import{t as a}from"./src-Bvgh6dl3.js";function o({activePath:e,onNavigate:t}){function n(t){return t===`/`?e===`/`:e.startsWith(t)}return(0,c.jsxs)(`aside`,{className:`flex w-56 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground h-[400px]`,children:[(0,c.jsx)(`div`,{className:`flex h-12 items-center px-4 border-b`,children:(0,c.jsx)(`span`,{className:`text-sm font-semibold`,children:`Control Plane`})}),(0,c.jsx)(`nav`,{className:`flex-1 px-2 py-2`,children:(0,c.jsx)(`ul`,{className:`flex flex-col gap-0.5`,children:l.map(e=>(0,c.jsx)(`li`,{children:(0,c.jsx)(`button`,{onClick:()=>t?.(e.path),className:i(`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors outline-none border-none cursor-pointer bg-transparent`,n(e.path)?`bg-sidebar-accent text-sidebar-accent-foreground font-medium`:`text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground`),children:(0,c.jsx)(`span`,{children:e.label})})},e.path))})})]})}var s,c,l,u,d,f,p,m,h,g,_,v,y;t((()=>{s=e(n(),1),a(),c=r(),l=[{path:`/`,label:`Dashboard`},{path:`/workspaces`,label:`Workspaces`},{path:`/users`,label:`Users`},{path:`/events`,label:`Events`},{path:`/overrides`,label:`Overrides`},{path:`/upsells`,label:`Upsells`}],u={title:`Ctrl/AdminSidebar`,component:o,tags:[`autodocs`]},d={args:{activePath:`/`}},f={args:{activePath:`/workspaces`}},p={args:{activePath:`/workspaces/ws-123`}},m={args:{activePath:`/users`}},h={args:{activePath:`/events`}},g={args:{activePath:`/overrides`}},_={args:{activePath:`/upsells`}},v={render:()=>{let[e,t]=(0,s.useState)(`/`);return(0,c.jsx)(o,{activePath:e,onNavigate:t})}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    activePath: "/"
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    activePath: "/workspaces"
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    activePath: "/workspaces/ws-123"
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    activePath: "/users"
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    activePath: "/events"
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    activePath: "/overrides"
  }
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  args: {
    activePath: "/upsells"
  }
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [active, setActive] = useState("/");
    return <AdminSidebar activePath={active} onNavigate={setActive} />;
  }
}`,...v.parameters?.docs?.source}}},y=[`Dashboard`,`Workspaces`,`WorkspaceDetail`,`Users`,`Events`,`Overrides`,`Upsells`,`Interactive`]}))();export{d as Dashboard,h as Events,v as Interactive,g as Overrides,_ as Upsells,m as Users,p as WorkspaceDetail,f as Workspaces,y as __namedExportsOrder,u as default};