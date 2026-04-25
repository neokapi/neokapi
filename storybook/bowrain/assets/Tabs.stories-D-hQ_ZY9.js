import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{C as n,S as r,b as i,t as a,x as o}from"./src-Bvgh6dl3.js";var s,c,l,u,d;e((()=>{a(),s=t(),c={title:`Foundations/Tabs`,component:i,tags:[`autodocs`],decorators:[e=>(0,s.jsx)(`div`,{style:{maxWidth:500,padding:16},children:(0,s.jsx)(e,{})})]},l={render:()=>(0,s.jsxs)(i,{defaultValue:`source`,children:[(0,s.jsxs)(r,{children:[(0,s.jsx)(n,{value:`source`,children:`Source`}),(0,s.jsx)(n,{value:`target`,children:`Target`}),(0,s.jsx)(n,{value:`preview`,children:`Preview`})]}),(0,s.jsx)(o,{value:`source`,children:(0,s.jsx)(`p`,{className:`text-sm text-muted-foreground p-4`,children:`Source text content`})}),(0,s.jsx)(o,{value:`target`,children:(0,s.jsx)(`p`,{className:`text-sm text-muted-foreground p-4`,children:`Target translation content`})}),(0,s.jsx)(o,{value:`preview`,children:(0,s.jsx)(`p`,{className:`text-sm text-muted-foreground p-4`,children:`Formatted preview`})})]})},u={render:()=>(0,s.jsxs)(i,{defaultValue:`tm`,children:[(0,s.jsxs)(r,{children:[(0,s.jsx)(n,{value:`tm`,children:`Translation Memory`}),(0,s.jsx)(n,{value:`terms`,children:`Terminology`})]}),(0,s.jsx)(o,{value:`tm`,children:(0,s.jsx)(`p`,{className:`text-sm text-muted-foreground p-4`,children:`TM matches will appear here`})}),(0,s.jsx)(o,{value:`terms`,children:(0,s.jsx)(`p`,{className:`text-sm text-muted-foreground p-4`,children:`Term suggestions will appear here`})})]})},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  render: () => <Tabs defaultValue="source">
      <TabsList>
        <TabsTrigger value="source">Source</TabsTrigger>
        <TabsTrigger value="target">Target</TabsTrigger>
        <TabsTrigger value="preview">Preview</TabsTrigger>
      </TabsList>
      <TabsContent value="source">
        <p className="text-sm text-muted-foreground p-4">Source text content</p>
      </TabsContent>
      <TabsContent value="target">
        <p className="text-sm text-muted-foreground p-4">Target translation content</p>
      </TabsContent>
      <TabsContent value="preview">
        <p className="text-sm text-muted-foreground p-4">Formatted preview</p>
      </TabsContent>
    </Tabs>
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  render: () => <Tabs defaultValue="tm">
      <TabsList>
        <TabsTrigger value="tm">Translation Memory</TabsTrigger>
        <TabsTrigger value="terms">Terminology</TabsTrigger>
      </TabsList>
      <TabsContent value="tm">
        <p className="text-sm text-muted-foreground p-4">TM matches will appear here</p>
      </TabsContent>
      <TabsContent value="terms">
        <p className="text-sm text-muted-foreground p-4">Term suggestions will appear here</p>
      </TabsContent>
    </Tabs>
}`,...u.parameters?.docs?.source}}},d=[`Default`,`Glass`]}))();export{l as Default,u as Glass,d as __namedExportsOrder,c as default};