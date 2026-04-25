import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,t as a}from"./ThemeContext-DQWB99f7.js";import{n as o,t as s}from"./AppShell-CpYHEFl7.js";import{a as c,i as l,n as u,t as d}from"./WorkspaceContext-BoGcQSoJ.js";import{n as f,t as p}from"./mock-adapter-S-P09mVY.js";var m,h,g,_,v,y,b,x,S,C,w,T,E,D;t((()=>{m=e(n(),1),o(),i(),c(),u(),f(),h=r(),{fn:g}=__STORYBOOK_MODULE_TEST__,_={id:`ws-1`,name:`Demo Workspace`,slug:`demo`,description:``,logo_url:``,type:`personal`,role:`owner`},v={id:`ws-2`,name:`Acme Corp`,slug:`acme`,description:``,logo_url:``,type:`team`,role:`editor`},y={id:`proj-1`,name:`Marketing Website`,default_source_language:`en-US`,target_languages:[`fr-FR`,`de-DE`,`ja-JP`,`es-ES`],workspace_id:`ws-1`,items:[{id:`itm-lnd`,name:`landing.html`,format:`html`,type:`file`,size:12e3,block_count:48,word_count:1320},{id:`itm-abt`,name:`about.json`,format:`json`,type:`file`,size:4200,block_count:22,word_count:280},{id:`itm-faq`,name:`faq.md`,format:`md`,type:`file`,size:8e3,block_count:35,word_count:620},{id:`itm-prc`,name:`pricing.xliff`,format:`xliff`,type:`file`,size:6e3,block_count:18,word_count:190}],streams:[{name:`main`,parent:``,base_cursor:0,archived:!1,visibility:`public`,description:``,created_at:`2025-12-01T10:00:00Z`,created_by:`user-1`},{name:`q1-campaign`,parent:`main`,base_cursor:5,archived:!1,visibility:`public`,description:`Q1 campaign`,created_at:`2026-02-01T10:00:00Z`,created_by:`user-1`},{name:`review/alice`,parent:`main`,base_cursor:3,archived:!1,visibility:`shared`,description:`Alice's review`,created_at:`2026-03-01T10:00:00Z`,created_by:`user-2`,shared_with:[`user-1`]}],created_at:`2025-12-01T10:00:00Z`,modified_at:`2026-03-13T09:15:00Z`},b={title:`Components/AppShell`,component:s,tags:[`autodocs`],decorators:[e=>(0,h.jsx)(a,{children:(0,h.jsx)(l,{adapter:p(),children:(0,h.jsx)(d,{initialWorkspace:_,children:(0,h.jsx)(`div`,{style:{width:`100vw`,height:`100vh`,overflow:`hidden`},children:(0,h.jsx)(e,{})})})})})],parameters:{layout:`fullscreen`}},x={render:()=>{let[e,t]=(0,m.useState)(!1),[n,r]=(0,m.useState)(`translate`);return(0,h.jsx)(s,{workspaces:[_,v],activeWorkspace:_,onSelectWorkspace:()=>{},onCreateWorkspace:()=>{},activeView:n,onViewChange:r,user:{id:`u-1`,email:`user@example.com`,name:`Demo User`,avatar_url:``},onSignOut:()=>{},collapsed:e,onCollapsedChange:t,sidebarContext:{level:`workspace`,activeView:n},children:(0,h.jsx)(`div`,{className:`flex items-center justify-center h-full text-muted-foreground`,children:`Workspace Home — Projects Dashboard`})})}},S={render:()=>{let[e,t]=(0,m.useState)(!1),n={level:`project`,project:y,activeStream:`main`,activeProjectView:`dashboard`,onBack:g(),onOpenDashboard:g(),onOpenFile:g(),onStreamChange:g(),onCreateStream:g(),onOpenAutomations:g()};return(0,h.jsx)(s,{workspaces:[_,v],activeWorkspace:_,onSelectWorkspace:()=>{},activeView:`translate`,onViewChange:()=>{},user:{id:`u-1`,email:`user@example.com`,name:`Demo User`,avatar_url:``},collapsed:e,onCollapsedChange:t,sidebarContext:n,children:(0,h.jsx)(`div`,{className:`flex items-center justify-center h-full text-muted-foreground`,children:`Project Detail — File list and upload area`})})}},C={render:()=>{let[e,t]=(0,m.useState)(!1),n={level:`project`,project:y,activeStream:`main`,activeProjectView:`automations`,onBack:g(),onOpenDashboard:g(),onOpenFile:g(),onStreamChange:g(),onOpenAutomations:g()};return(0,h.jsx)(s,{workspaces:[_,v],activeWorkspace:_,onSelectWorkspace:()=>{},activeView:`translate`,onViewChange:()=>{},user:{id:`u-1`,email:`user@example.com`,name:`Demo User`,avatar_url:``},collapsed:e,onCollapsedChange:t,sidebarContext:n,children:(0,h.jsx)(`div`,{className:`flex items-center justify-center h-full text-muted-foreground`,children:`Project Automations`})})}},w={render:()=>{let[e,t]=(0,m.useState)(!1),n={level:`project`,project:y,activeStream:`main`,activeProjectView:`dashboard`,onBack:g(),onOpenDashboard:g(),onOpenFile:g(),onStreamChange:g(),onOpenAutomations:g()};return(0,h.jsx)(s,{workspaces:[_,v],activeWorkspace:_,onSelectWorkspace:()=>{},activeView:`translate`,onViewChange:()=>{},user:{id:`u-1`,email:`user@example.com`,name:`Demo User`,avatar_url:``},collapsed:e,onCollapsedChange:t,sidebarContext:n,children:(0,h.jsx)(`div`,{className:`flex items-center justify-center h-full text-muted-foreground`,children:`Translation Editor — about.json (fr-FR)`})})}},T={render:()=>{let[e,t]=(0,m.useState)(!0),n={level:`project`,project:y,activeStream:`main`,activeProjectView:`dashboard`,onBack:g(),onOpenDashboard:g(),onOpenFile:g(),onStreamChange:g()};return(0,h.jsx)(s,{workspaces:[_],activeWorkspace:_,onSelectWorkspace:()=>{},activeView:`translate`,onViewChange:()=>{},user:null,collapsed:e,onCollapsedChange:t,sidebarContext:n,children:(0,h.jsx)(`div`,{className:`flex items-center justify-center h-full text-muted-foreground`,children:`Collapsed sidebar — project mode`})})}},E={render:()=>{let[e,t]=(0,m.useState)(!0),[n,r]=(0,m.useState)(`translate`);return(0,h.jsx)(s,{workspaces:[_],activeWorkspace:_,onSelectWorkspace:()=>{},activeView:n,onViewChange:r,user:null,collapsed:e,onCollapsedChange:t,children:(0,h.jsx)(`div`,{className:`flex items-center justify-center h-full text-muted-foreground`,children:`Collapsed sidebar — workspace mode`})})}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [collapsed, setCollapsed] = useState(false);
    const [view, setView] = useState("translate");
    return <AppShell workspaces={[mockWorkspace, secondWorkspace]} activeWorkspace={mockWorkspace} onSelectWorkspace={() => {}} onCreateWorkspace={() => {}} activeView={view} onViewChange={setView} user={{
      id: "u-1",
      email: "user@example.com",
      name: "Demo User",
      avatar_url: ""
    }} onSignOut={() => {}} collapsed={collapsed} onCollapsedChange={setCollapsed} sidebarContext={{
      level: "workspace",
      activeView: view as "translate"
    }}>
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Workspace Home — Projects Dashboard
        </div>
      </AppShell>;
  }
}`,...x.parameters?.docs?.source},description:{story:`Workspace-level: default flat navigation with Projects, Brand Voice, Termbase, etc.`,...x.parameters?.docs?.description}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [collapsed, setCollapsed] = useState(false);
    const ctx: SidebarContext = {
      level: "project",
      project: sampleProject,
      activeStream: "main",
      activeProjectView: "dashboard",
      onBack: fn(),
      onOpenDashboard: fn(),
      onOpenFile: fn(),
      onStreamChange: fn(),
      onCreateStream: fn(),
      onOpenAutomations: fn()
    };
    return <AppShell workspaces={[mockWorkspace, secondWorkspace]} activeWorkspace={mockWorkspace} onSelectWorkspace={() => {}} activeView="translate" onViewChange={() => {}} user={{
      id: "u-1",
      email: "user@example.com",
      name: "Demo User",
      avatar_url: ""
    }} collapsed={collapsed} onCollapsedChange={setCollapsed} sidebarContext={ctx}>
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Project Detail — File list and upload area
        </div>
      </AppShell>;
  }
}`,...S.parameters?.docs?.source},description:{story:`Project-level: sidebar shows Dashboard (active) and Automations.`,...S.parameters?.docs?.description}}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [collapsed, setCollapsed] = useState(false);
    const ctx: SidebarContext = {
      level: "project",
      project: sampleProject,
      activeStream: "main",
      activeProjectView: "automations",
      onBack: fn(),
      onOpenDashboard: fn(),
      onOpenFile: fn(),
      onStreamChange: fn(),
      onOpenAutomations: fn()
    };
    return <AppShell workspaces={[mockWorkspace, secondWorkspace]} activeWorkspace={mockWorkspace} onSelectWorkspace={() => {}} activeView="translate" onViewChange={() => {}} user={{
      id: "u-1",
      email: "user@example.com",
      name: "Demo User",
      avatar_url: ""
    }} collapsed={collapsed} onCollapsedChange={setCollapsed} sidebarContext={ctx}>
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Project Automations
        </div>
      </AppShell>;
  }
}`,...C.parameters?.docs?.source},description:{story:`Automations page: sidebar highlights Automations, Home shows project name.`,...C.parameters?.docs?.description}}},w.parameters={...w.parameters,docs:{...w.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [collapsed, setCollapsed] = useState(false);
    const ctx: SidebarContext = {
      level: "project",
      project: sampleProject,
      activeStream: "main",
      activeProjectView: "dashboard",
      onBack: fn(),
      onOpenDashboard: fn(),
      onOpenFile: fn(),
      onStreamChange: fn(),
      onOpenAutomations: fn()
    };
    return <AppShell workspaces={[mockWorkspace, secondWorkspace]} activeWorkspace={mockWorkspace} onSelectWorkspace={() => {}} activeView="translate" onViewChange={() => {}} user={{
      id: "u-1",
      email: "user@example.com",
      name: "Demo User",
      avatar_url: ""
    }} collapsed={collapsed} onCollapsedChange={setCollapsed} sidebarContext={ctx}>
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Translation Editor — about.json (fr-FR)
        </div>
      </AppShell>;
  }
}`,...w.parameters?.docs?.source},description:{story:`Editor-level: sidebar shows Dashboard and Automations, Home shows project name.`,...w.parameters?.docs?.description}}},T.parameters={...T.parameters,docs:{...T.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [collapsed, setCollapsed] = useState(true);
    const ctx: SidebarContext = {
      level: "project",
      project: sampleProject,
      activeStream: "main",
      activeProjectView: "dashboard",
      onBack: fn(),
      onOpenDashboard: fn(),
      onOpenFile: fn(),
      onStreamChange: fn()
    };
    return <AppShell workspaces={[mockWorkspace]} activeWorkspace={mockWorkspace} onSelectWorkspace={() => {}} activeView="translate" onViewChange={() => {}} user={null} collapsed={collapsed} onCollapsedChange={setCollapsed} sidebarContext={ctx}>
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Collapsed sidebar — project mode
        </div>
      </AppShell>;
  }
}`,...T.parameters?.docs?.source},description:{story:`Project-level with sidebar collapsed to icon mode.`,...T.parameters?.docs?.description}}},E.parameters={...E.parameters,docs:{...E.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [collapsed, setCollapsed] = useState(true);
    const [view, setView] = useState("translate");
    return <AppShell workspaces={[mockWorkspace]} activeWorkspace={mockWorkspace} onSelectWorkspace={() => {}} activeView={view} onViewChange={setView} user={null} collapsed={collapsed} onCollapsedChange={setCollapsed}>
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Collapsed sidebar — workspace mode
        </div>
      </AppShell>;
  }
}`,...E.parameters?.docs?.source},description:{story:`Workspace-level collapsed.`,...E.parameters?.docs?.description}}},D=[`WorkspaceLevel`,`ProjectLevel`,`AutomationsLevel`,`EditorLevel`,`ProjectCollapsed`,`WorkspaceCollapsed`]}))();export{C as AutomationsLevel,w as EditorLevel,T as ProjectCollapsed,S as ProjectLevel,E as WorkspaceCollapsed,x as WorkspaceLevel,D as __namedExportsOrder,b as default};