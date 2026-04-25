import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{t as i}from"./src-Q72DuJW2.js";import{t as a}from"./button-MBstDm8k.js";import{d as o,f as s,h as c,m as l,p as u,u as d}from"./command-B-cloO7Q.js";function f({open:e,onOpenChange:t,currentPlan:n}){let[r,i]=(0,p.useState)(n);return(0,m.jsx)(d,{open:e,onOpenChange:t,children:(0,m.jsxs)(o,{className:`sm:max-w-[480px]`,children:[(0,m.jsxs)(l,{children:[(0,m.jsx)(c,{children:`Change Plan`}),(0,m.jsxs)(s,{children:[`Current plan: `,n,`. Select a new plan for this workspace.`]})]}),(0,m.jsx)(`div`,{className:`space-y-4 py-2`,children:(0,m.jsxs)(`div`,{className:`space-y-2`,children:[(0,m.jsx)(`label`,{className:`text-sm font-medium`,children:`Select Plan`}),(0,m.jsx)(`div`,{className:`grid grid-cols-2 gap-2`,children:h.map(e=>(0,m.jsx)(`button`,{onClick:()=>i(e),className:`rounded-md border px-3 py-2 text-sm capitalize cursor-pointer transition-colors ${r===e?`border-primary bg-primary/10 text-primary font-medium`:`border-border hover:bg-muted`}`,children:e},e))})]})}),(0,m.jsxs)(u,{children:[(0,m.jsx)(a,{variant:`outline`,onClick:()=>t(!1),children:`Cancel`}),(0,m.jsx)(a,{disabled:r===n,children:`Change Plan`})]})]})})}var p,m,h,g,_,v,y,b,x,S;t((()=>{i(),p=e(n(),1),m=r(),h=[`free`,`pro`,`team`,`enterprise`],g={title:`Ctrl/ChangePlanDialog`,component:f,tags:[`autodocs`]},_={args:{open:!0,onOpenChange:()=>{},workspaceId:`ws-1`,currentPlan:`free`}},v={args:{open:!0,onOpenChange:()=>{},workspaceId:`ws-1`,currentPlan:`pro`}},y={args:{open:!0,onOpenChange:()=>{},workspaceId:`ws-1`,currentPlan:`team`}},b={args:{open:!0,onOpenChange:()=>{},workspaceId:`ws-1`,currentPlan:`enterprise`}},x={render:()=>{let[e,t]=(0,p.useState)(!1);return(0,m.jsxs)(`div`,{children:[(0,m.jsx)(a,{onClick:()=>t(!0),children:`Change Plan`}),(0,m.jsx)(f,{open:e,onOpenChange:t,workspaceId:`ws-1`,currentPlan:`free`})]})}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: () => {},
    workspaceId: "ws-1",
    currentPlan: "free"
  }
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: () => {},
    workspaceId: "ws-1",
    currentPlan: "pro"
  }
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: () => {},
    workspaceId: "ws-1",
    currentPlan: "team"
  }
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: () => {},
    workspaceId: "ws-1",
    currentPlan: "enterprise"
  }
}`,...b.parameters?.docs?.source}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [open, setOpen] = useState(false);
    return <div>
        <Button onClick={() => setOpen(true)}>Change Plan</Button>
        <ChangePlanDialog open={open} onOpenChange={setOpen} workspaceId="ws-1" currentPlan="free" />
      </div>;
  }
}`,...x.parameters?.docs?.source}}},S=[`FromFree`,`FromPro`,`FromTeam`,`FromEnterprise`,`Interactive`]}))();export{b as FromEnterprise,_ as FromFree,v as FromPro,y as FromTeam,x as Interactive,S as __namedExportsOrder,g as default};