import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{t as n}from"./src-Q72DuJW2.js";import{t as r}from"./button-MBstDm8k.js";import{d as i,f as a,g as o,h as s,m as c,p as l,u}from"./command-B-cloO7Q.js";var d,f,p,m,h;e((()=>{n(),d=t(),f={title:`Foundations/Dialog`,component:u,tags:[`autodocs`]},p={render:()=>(0,d.jsxs)(u,{children:[(0,d.jsx)(o,{asChild:!0,children:(0,d.jsx)(r,{variant:`outline`,children:`Open Dialog`})}),(0,d.jsxs)(i,{children:[(0,d.jsxs)(c,{children:[(0,d.jsx)(s,{children:`Create Project`}),(0,d.jsx)(a,{children:`Set up a new localization project. You can configure languages and formats after creation.`})]}),(0,d.jsx)(`p`,{className:`text-sm text-muted-foreground`,children:`Project form goes here.`}),(0,d.jsx)(l,{children:(0,d.jsx)(r,{children:`Create`})})]})]})},m={render:()=>(0,d.jsxs)(u,{children:[(0,d.jsx)(o,{asChild:!0,children:(0,d.jsx)(r,{variant:`outline`,children:`Open Dialog`})}),(0,d.jsxs)(i,{showCloseButton:!1,children:[(0,d.jsxs)(c,{children:[(0,d.jsx)(s,{children:`Confirm Action`}),(0,d.jsx)(a,{children:`Are you sure you want to push these changes?`})]}),(0,d.jsx)(l,{showCloseButton:!0,children:(0,d.jsx)(r,{children:`Confirm`})})]})]})},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  render: () => <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline">Open Dialog</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create Project</DialogTitle>
          <DialogDescription>
            Set up a new localization project. You can configure languages and formats after
            creation.
          </DialogDescription>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">Project form goes here.</p>
        <DialogFooter>
          <Button>Create</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline">Open Dialog</Button>
      </DialogTrigger>
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>Confirm Action</DialogTitle>
          <DialogDescription>Are you sure you want to push these changes?</DialogDescription>
        </DialogHeader>
        <DialogFooter showCloseButton>
          <Button>Confirm</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
}`,...m.parameters?.docs?.source}}},h=[`Default`,`WithCloseInFooter`]}))();export{p as Default,m as WithCloseInFooter,h as __namedExportsOrder,f as default};