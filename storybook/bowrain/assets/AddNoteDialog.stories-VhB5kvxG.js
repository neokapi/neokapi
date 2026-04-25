import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{t as i}from"./src-Q72DuJW2.js";import{t as a}from"./button-MBstDm8k.js";import{d as o,f as s,h as c,m as l,p as u,u as d}from"./command-B-cloO7Q.js";function f({open:e,onOpenChange:t}){let[n,r]=(0,p.useState)(``);return(0,m.jsx)(d,{open:e,onOpenChange:t,children:(0,m.jsxs)(o,{className:`sm:max-w-[480px]`,children:[(0,m.jsxs)(l,{children:[(0,m.jsx)(c,{children:`Add Internal Note`}),(0,m.jsx)(s,{children:`Add a note visible only to admins. Useful for tracking support interactions and decisions.`})]}),(0,m.jsx)(`div`,{className:`space-y-4 py-2`,children:(0,m.jsxs)(`div`,{className:`space-y-2`,children:[(0,m.jsx)(`label`,{htmlFor:`note-content`,className:`text-sm font-medium`,children:`Note`}),(0,m.jsx)(`textarea`,{id:`note-content`,className:`w-full min-h-[100px] rounded-md border bg-background px-3 py-2 text-sm resize-y`,placeholder:`e.g. Customer called about upgrade options. Offered 2-week Pro trial.`,value:n,onChange:e=>r(e.target.value)})]})}),(0,m.jsxs)(u,{children:[(0,m.jsx)(a,{variant:`outline`,onClick:()=>t(!1),children:`Cancel`}),(0,m.jsx)(a,{disabled:!n.trim(),children:`Add Note`})]})]})})}var p,m,h,g,_,v;t((()=>{i(),p=e(n(),1),m=r(),h={title:`Ctrl/AddNoteDialog`,component:f,tags:[`autodocs`]},g={args:{open:!0,onOpenChange:()=>{},workspaceId:`ws-1`}},_={render:()=>{let[e,t]=(0,p.useState)(!1);return(0,m.jsxs)(`div`,{children:[(0,m.jsx)(a,{onClick:()=>t(!0),children:`Add Note`}),(0,m.jsx)(f,{open:e,onOpenChange:t,workspaceId:`ws-1`})]})}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: () => {},
    workspaceId: "ws-1"
  }
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [open, setOpen] = useState(false);
    return <div>
        <Button onClick={() => setOpen(true)}>Add Note</Button>
        <AddNoteDialog open={open} onOpenChange={setOpen} workspaceId="ws-1" />
      </div>;
  }
}`,..._.parameters?.docs?.source}}},v=[`Open`,`Interactive`]}))();export{_ as Interactive,g as Open,v as __namedExportsOrder,h as default};