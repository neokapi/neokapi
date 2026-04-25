import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./WorkspaceIcon-BKlrXijj.js";import{D as i,f as a,s as o}from"./fixtures-nZqLq3hB.js";var s,c,l,u,d,f,p,m,h,g;e((()=>{n(),o(),s=t(),{fn:c}=__STORYBOOK_MODULE_TEST__,l={title:`Workspace/Settings/WorkspaceIcon`,component:r,tags:[`autodocs`],decorators:[e=>(0,s.jsx)(`div`,{style:{padding:24,display:`flex`,gap:16,alignItems:`center`},children:(0,s.jsx)(e,{})})]},u={args:{workspace:i,active:!0,onClick:c()}},d={args:{workspace:a,active:!1,onClick:c()}},f={args:{workspace:i,active:!0,onClick:c(),size:56}},p={args:{workspace:a,active:!0,onClick:c()}},m={args:{workspace:a,active:!1,onClick:c(),size:56}},h={render:()=>(0,s.jsxs)(`div`,{style:{display:`flex`,gap:24,alignItems:`center`},children:[(0,s.jsx)(r,{workspace:i,active:!1,onClick:c()}),(0,s.jsx)(r,{workspace:a,active:!1,onClick:c()}),(0,s.jsx)(r,{workspace:i,active:!0,onClick:c()}),(0,s.jsx)(r,{workspace:a,active:!0,onClick:c()}),(0,s.jsx)(r,{workspace:a,active:!1,onClick:c(),size:56})]})},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    workspace: sampleWorkspace,
    active: true,
    onClick: fn()
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    workspace: personalWorkspace,
    active: false,
    onClick: fn()
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    workspace: sampleWorkspace,
    active: true,
    onClick: fn(),
    size: 56
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    workspace: personalWorkspace,
    active: true,
    onClick: fn()
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    workspace: personalWorkspace,
    active: false,
    onClick: fn(),
    size: 56
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    display: "flex",
    gap: 24,
    alignItems: "center"
  }}>
      <WorkspaceIcon workspace={sampleWorkspace} active={false} onClick={fn()} />
      <WorkspaceIcon workspace={personalWorkspace} active={false} onClick={fn()} />
      <WorkspaceIcon workspace={sampleWorkspace} active={true} onClick={fn()} />
      <WorkspaceIcon workspace={personalWorkspace} active={true} onClick={fn()} />
      <WorkspaceIcon workspace={personalWorkspace} active={false} onClick={fn()} size={56} />
    </div>
}`,...h.parameters?.docs?.source}}},g=[`Active`,`Inactive`,`Large`,`PersonalActive`,`PersonalLarge`,`AllVariants`]}))();export{u as Active,h as AllVariants,d as Inactive,f as Large,p as PersonalActive,m as PersonalLarge,g as __namedExportsOrder,l as default};