import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,t as a}from"./BulkActionBar-oXW8WMaj.js";var o,s,c,l,u,d,f,p,m;t((()=>{o=e(n(),1),i(),s=r(),c={title:`Resource Browser/BulkActionBar`,component:a,tags:[`autodocs`],parameters:{docs:{description:{component:`Floating bottom bar that appears when items are selected. Shows selection count and bulk action buttons. Delete requires a two-click confirmation.`}}}},l={args:{selectedCount:3,onDelete:()=>{},onDeselectAll:()=>{}}},u={args:{selectedCount:42,onDelete:()=>{},onDeselectAll:()=>{}}},d={args:{selectedCount:12,onDelete:()=>{},onAnnotateEntities:()=>{},onDeselectAll:()=>{}}},f={args:{selectedCount:5,onDelete:()=>{},confirmDelete:!0,onDeselectAll:()=>{}}},p={render:function(){let[e,t]=(0,o.useState)(!1),[n,r]=(0,o.useState)(7);return(0,s.jsxs)(`div`,{children:[(0,s.jsx)(`p`,{className:`mb-4 text-sm text-muted-foreground`,children:`Click Delete to see the confirmation state. Click Cancel or Deselect all to reset.`}),(0,s.jsx)(a,{selectedCount:n,onDelete:()=>{e?(r(0),t(!1)):t(!0)},confirmDelete:e,onAnnotateEntities:()=>{},onDeselectAll:()=>{r(0),t(!1)}})]})}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    selectedCount: 3,
    onDelete: () => {},
    onDeselectAll: () => {}
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    selectedCount: 42,
    onDelete: () => {},
    onDeselectAll: () => {}
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    selectedCount: 12,
    onDelete: () => {},
    onAnnotateEntities: () => {},
    onDeselectAll: () => {}
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    selectedCount: 5,
    onDelete: () => {},
    confirmDelete: true,
    onDeselectAll: () => {}
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  render: function InteractiveBulkAction() {
    const [confirmDelete, setConfirmDelete] = useState(false);
    const [count, setCount] = useState(7);
    return <div>
        <p className="mb-4 text-sm text-muted-foreground">
          Click Delete to see the confirmation state. Click Cancel or Deselect all to reset.
        </p>
        <BulkActionBar selectedCount={count} onDelete={() => {
        if (confirmDelete) {
          setCount(0);
          setConfirmDelete(false);
        } else {
          setConfirmDelete(true);
        }
      }} confirmDelete={confirmDelete} onAnnotateEntities={() => {}} onDeselectAll={() => {
        setCount(0);
        setConfirmDelete(false);
      }} />
      </div>;
  }
}`,...p.parameters?.docs?.source},description:{story:`Interactive demo showing the full delete confirmation flow.`,...p.parameters?.docs?.description}}},m=[`FewSelected`,`ManySelected`,`WithAnnotateEntities`,`ConfirmDelete`,`Interactive`]}))();export{f as ConfirmDelete,l as FewSelected,p as Interactive,u as ManySelected,d as WithAnnotateEntities,m as __namedExportsOrder,c as default};