import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{t as i}from"./src-Bvgh6dl3.js";import{t as a}from"./button-MBstDm8k.js";import{t as o}from"./input-C1MWkU_r.js";import{d as s,f as c,h as l,m as u,p as d,u as f}from"./command-B-cloO7Q.js";function p({open:e,onOpenChange:t}){let[n,r]=(0,m.useState)(``),[i,p]=(0,m.useState)(``);return(0,h.jsx)(f,{open:e,onOpenChange:t,children:(0,h.jsxs)(s,{className:`sm:max-w-[480px]`,children:[(0,h.jsxs)(u,{children:[(0,h.jsx)(l,{children:`Grant Credits`}),(0,h.jsx)(c,{children:`Grant bonus credits to this workspace. This will be recorded in the credit ledger.`})]}),(0,h.jsxs)(`div`,{className:`space-y-4 py-2`,children:[(0,h.jsxs)(`div`,{className:`space-y-2`,children:[(0,h.jsx)(`label`,{htmlFor:`amount`,className:`text-sm font-medium`,children:`Amount (tokens)`}),(0,h.jsx)(o,{id:`amount`,type:`number`,min:`1`,placeholder:`e.g. 200000`,value:n,onChange:e=>r(e.target.value)})]}),(0,h.jsxs)(`div`,{className:`space-y-2`,children:[(0,h.jsx)(`label`,{htmlFor:`reason`,className:`text-sm font-medium`,children:`Reason`}),(0,h.jsx)(o,{id:`reason`,placeholder:`e.g. Support compensation for outage`,value:i,onChange:e=>p(e.target.value)})]})]}),(0,h.jsxs)(d,{children:[(0,h.jsx)(a,{variant:`outline`,onClick:()=>t(!1),children:`Cancel`}),(0,h.jsx)(a,{disabled:!n||Number(n)<=0||!i,children:`Grant Credits`})]})]})})}var m,h,g,_,v,y;t((()=>{i(),m=e(n(),1),h=r(),g={title:`Ctrl/GrantCreditsDialog`,component:p,tags:[`autodocs`]},_={args:{open:!0,onOpenChange:()=>{},workspaceId:`ws-1`}},v={render:()=>{let[e,t]=(0,m.useState)(!1);return(0,h.jsxs)(`div`,{children:[(0,h.jsx)(a,{onClick:()=>t(!0),children:`Grant Credits`}),(0,h.jsx)(p,{open:e,onOpenChange:t,workspaceId:`ws-1`})]})}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: () => {},
    workspaceId: "ws-1"
  }
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [open, setOpen] = useState(false);
    return <div>
        <Button onClick={() => setOpen(true)}>Grant Credits</Button>
        <GrantCreditsDialog open={open} onOpenChange={setOpen} workspaceId="ws-1" />
      </div>;
  }
}`,...v.parameters?.docs?.source}}},y=[`Open`,`Interactive`]}))();export{v as Interactive,_ as Open,y as __namedExportsOrder,g as default};