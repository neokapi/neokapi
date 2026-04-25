import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{A as n,B as r,D as i,F as a,I as o,L as s,M as c,N as l,O as u,P as d,R as f,j as p,k as m,t as h}from"./src-Q72DuJW2.js";var g,_,v,y,b;e((()=>{h(),g=t(),_={title:`Foundations/Sidebar`,component:i,tags:[`autodocs`]},v=[{title:`Dashboard`,url:`#`},{title:`Projects`,url:`#`},{title:`Translation Memory`,url:`#`},{title:`Terminology`,url:`#`},{title:`Settings`,url:`#`}],y={render:()=>(0,g.jsxs)(f,{children:[(0,g.jsxs)(i,{children:[(0,g.jsx)(l,{children:(0,g.jsx)(`span`,{className:`px-2 text-sm font-semibold`,children:`Bowrain`})}),(0,g.jsx)(u,{children:(0,g.jsxs)(n,{children:[(0,g.jsx)(c,{children:`Workspace`}),(0,g.jsx)(p,{children:(0,g.jsx)(a,{children:v.map(e=>(0,g.jsx)(s,{children:(0,g.jsx)(o,{children:(0,g.jsx)(`span`,{children:e.title})})},e.title))})})]})}),(0,g.jsx)(m,{children:(0,g.jsx)(`span`,{className:`px-2 text-xs text-muted-foreground`,children:`v1.0.0`})})]}),(0,g.jsxs)(d,{children:[(0,g.jsxs)(`header`,{className:`flex h-12 items-center gap-2 border-b px-4`,children:[(0,g.jsx)(r,{}),(0,g.jsx)(`span`,{className:`text-sm font-medium`,children:`Main Content`})]}),(0,g.jsx)(`div`,{className:`p-4`,children:(0,g.jsx)(`p`,{className:`text-sm text-muted-foreground`,children:`Page content goes here.`})})]})]})},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  render: () => <SidebarProvider>
      <Sidebar>
        <SidebarHeader>
          <span className="px-2 text-sm font-semibold">Bowrain</span>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>Workspace</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {items.map(item => <SidebarMenuItem key={item.title}>
                    <SidebarMenuButton>
                      <span>{item.title}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>)}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter>
          <span className="px-2 text-xs text-muted-foreground">v1.0.0</span>
        </SidebarFooter>
      </Sidebar>
      <SidebarInset>
        <header className="flex h-12 items-center gap-2 border-b px-4">
          <SidebarTrigger />
          <span className="text-sm font-medium">Main Content</span>
        </header>
        <div className="p-4">
          <p className="text-sm text-muted-foreground">Page content goes here.</p>
        </div>
      </SidebarInset>
    </SidebarProvider>
}`,...y.parameters?.docs?.source}}},b=[`Default`]}))();export{y as Default,b as __namedExportsOrder,_ as default};