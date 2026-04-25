import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{n as o,t as s}from"./BulkActionBar-CPbzf0Dj.js";var c,l,u,d,f,p,m,h,g;t((()=>{i(),c=e(n(),1),o(),l=r(),u={title:`Resource Browser/BulkActionBar`,component:s,tags:[`autodocs`],parameters:{docs:{description:{component:`Floating bottom bar that appears when items are selected. Shows selection count and bulk action buttons. Delete requires a two-click confirmation.`}}}},d={args:{selectedCount:3,onDelete:()=>{},onDeselectAll:()=>{}}},f={args:{selectedCount:42,onDelete:()=>{},onDeselectAll:()=>{}}},p={args:{selectedCount:12,onDelete:()=>{},onAnnotateEntities:()=>{},onDeselectAll:()=>{}}},m={args:{selectedCount:5,onDelete:()=>{},confirmDelete:!0,onDeselectAll:()=>{}}},h={render:function(){let[e,t]=(0,c.useState)(!1),[n,r]=(0,c.useState)(7);return(0,l.jsxs)(`div`,{children:[(0,l.jsx)(`p`,{className:`mb-4 text-sm text-muted-foreground`,children:a(`2OYCq7`,`Click Delete to see the confirmation state. Click Cancel or Deselect all to reset.`)}),(0,l.jsx)(s,{selectedCount:n,onDelete:()=>{e?(r(0),t(!1)):t(!0)},confirmDelete:e,onAnnotateEntities:()=>{},onDeselectAll:()=>{r(0),t(!1)}})]})}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    selectedCount: 3,
    onDelete: () => {},
    onDeselectAll: () => {}
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    selectedCount: 42,
    onDelete: () => {},
    onDeselectAll: () => {}
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    selectedCount: 12,
    onDelete: () => {},
    onAnnotateEntities: () => {},
    onDeselectAll: () => {}
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    selectedCount: 5,
    onDelete: () => {},
    confirmDelete: true,
    onDeselectAll: () => {}
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
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
}`,...h.parameters?.docs?.source},description:{story:`Interactive demo showing the full delete confirmation flow.`,...h.parameters?.docs?.description}}},g=[`FewSelected`,`ManySelected`,`WithAnnotateEntities`,`ConfirmDelete`,`Interactive`]}))();export{m as ConfirmDelete,d as FewSelected,h as Interactive,f as ManySelected,p as WithAnnotateEntities,g as __namedExportsOrder,u as default};