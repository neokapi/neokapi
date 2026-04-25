import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,t as a}from"./AutomationRuleEditor-Be-t4qhV.js";import{k as o,l as s}from"./fixtures-D8OXoGLB.js";import{n as c,r as l}from"./decorators-BptasVUO.js";function u(){let[e,t]=(0,d.useState)(!0);return(0,f.jsxs)(`div`,{children:[(0,f.jsx)(`button`,{onClick:()=>t(!0),className:`px-3 py-1.5 text-sm border rounded`,children:`Open Editor`}),(0,f.jsx)(a,{open:e,onOpenChange:t,workspaceSlug:`demo`,projectId:`proj-demo-1`,onSave:e=>{console.log(`Saved:`,e),t(!1)}})]})}var d,f,p,m,h,g,_,v,y,b,x;t((()=>{d=e(n(),1),i(),c(),s(),f=r(),{fn:p}=__STORYBOOK_MODULE_TEST__,m={title:`Pages/Automation/AutomationRuleEditor`,component:a,tags:[`autodocs`],decorators:[l]},h={args:{open:!0,onOpenChange:p(),workspaceSlug:`demo`,projectId:`proj-demo-1`,onSave:p()}},g={args:{open:!0,onOpenChange:p(),workspaceSlug:`demo`,projectId:`proj-demo-1`,rule:o[0],onSave:p()}},_={args:{open:!0,onOpenChange:p(),workspaceSlug:`demo`,projectId:`proj-demo-1`,rule:o[3],onSave:p()}},v={args:{open:!0,onOpenChange:p(),workspaceSlug:`demo`,projectId:`proj-demo-1`,rule:o[4],onSave:p()}},y={args:{open:!0,onOpenChange:p(),workspaceSlug:`demo`,projectId:`proj-demo-1`,rule:o[0],onSave:p(),saving:!0}},b={render:()=>(0,f.jsx)(u,{})},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: fn(),
    workspaceSlug: "demo",
    projectId: "proj-demo-1",
    onSave: fn()
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: fn(),
    workspaceSlug: "demo",
    projectId: "proj-demo-1",
    rule: sampleAutomationRules[0],
    onSave: fn()
  }
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: fn(),
    workspaceSlug: "demo",
    projectId: "proj-demo-1",
    rule: sampleAutomationRules[3],
    // create_review_tasks rule
    onSave: fn()
  }
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: fn(),
    workspaceSlug: "demo",
    projectId: "proj-demo-1",
    rule: sampleAutomationRules[4],
    // create_source_review rule
    onSave: fn()
  }
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: fn(),
    workspaceSlug: "demo",
    projectId: "proj-demo-1",
    rule: sampleAutomationRules[0],
    onSave: fn(),
    saving: true
  }
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveWrapper />
}`,...b.parameters?.docs?.source}}},x=[`NewRule`,`EditExistingRule`,`EditWorkflowRule`,`EditSourceReviewRule`,`SavingState`,`Interactive`]}))();export{g as EditExistingRule,v as EditSourceReviewRule,_ as EditWorkflowRule,b as Interactive,h as NewRule,y as SavingState,x as __namedExportsOrder,m as default};