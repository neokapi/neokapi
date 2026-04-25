import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./TaskBoard-8Pn0uAv2.js";var i,a,o,s,c,l,u,d,f,p,m,h,g;e((()=>{n(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o=new Date,s=e=>new Date(o.getTime()-e*6e4).toISOString(),c=e=>new Date(o.getTime()+e*6e4).toISOString(),l=[{id:`task-1`,workspace_id:`ws-1`,project_id:`proj-1`,type:`translate`,status:`open`,priority:`high`,title:`Translate marketing page to French`,description:`The homepage and about page need French translations before the Q2 launch.`,assignee_id:`user-2`,created_by:`user-1`,due_at:c(1440),created_at:s(120),updated_at:s(60)},{id:`task-2`,workspace_id:`ws-1`,project_id:`proj-1`,type:`review`,status:`in_progress`,priority:`normal`,title:`Review German translations`,assignee_id:`user-3`,created_by:`user-1`,due_at:c(2880),created_at:s(240),updated_at:s(30)},{id:`task-3`,workspace_id:`ws-1`,project_id:`proj-1`,type:`fix_quality`,status:`open`,priority:`urgent`,title:`Fix QA issues in Japanese build`,description:`3 critical QA issues found in the latest Japanese translation batch.`,created_by:`user-1`,due_at:s(60),created_at:s(480),updated_at:s(480)},{id:`task-4`,workspace_id:`ws-1`,project_id:`proj-2`,type:`fix_brand_voice`,status:`open`,priority:`normal`,title:`Align mobile content with brand voice`,assignee_id:`user-2`,created_by:`user-1`,created_at:s(600),updated_at:s(600)},{id:`task-5`,workspace_id:`ws-1`,project_id:`proj-1`,type:`translate`,status:`completed`,priority:`normal`,title:`Translate onboarding flow to Spanish`,assignee_id:`user-2`,created_by:`user-1`,completed_by:`user-2`,created_at:s(10080),updated_at:s(2880),completed_at:s(2880)},{id:`task-6`,workspace_id:`ws-1`,project_id:`proj-1`,type:`review_terms`,status:`cancelled`,priority:`low`,title:`Review deprecated terms in glossary`,created_by:`user-1`,created_at:s(20160),updated_at:s(14400)},{id:`task-7`,workspace_id:`ws-1`,project_id:`proj-1`,type:`connector_setup`,status:`in_progress`,priority:`high`,title:`Configure GitHub connector for docs repo`,description:`Set up bidirectional sync with the documentation repository.`,assignee_id:`user-1`,created_by:`user-1`,created_at:s(1440),updated_at:s(60)},{id:`task-8`,workspace_id:`ws-1`,project_id:`proj-1`,type:`source_review`,status:`open`,priority:`normal`,title:`Review source content before translation`,description:`Check placeholders, terminology, and DNT tags before fan-out to target languages.`,assignee_id:`user-1`,created_by:`system`,data:{push_id:`push-abc`,items:`en.json`},created_at:s(30),updated_at:s(30)}],u={title:`Pages/Tasks/TaskBoard`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{maxWidth:960,padding:24},children:(0,i.jsx)(e,{})})]},d={args:{tasks:l,currentUserId:`user-1`,onCompleteTask:a(),onCancelTask:a(),onTaskClick:a()}},f={args:{tasks:[]}},p={args:{tasks:[],loading:!0}},m={args:{tasks:l.filter(e=>e.status===`open`),onCompleteTask:a(),onCancelTask:a(),onTaskClick:a()}},h={args:{tasks:l.filter(e=>e.status===`open`||e.status===`in_progress`),onCompleteTask:a(),onCancelTask:a(),onTaskClick:a()}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    tasks: sampleTasks,
    currentUserId: "user-1",
    onCompleteTask: fn(),
    onCancelTask: fn(),
    onTaskClick: fn()
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    tasks: []
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    tasks: [],
    loading: true
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    tasks: sampleTasks.filter(t => t.status === "open"),
    onCompleteTask: fn(),
    onCancelTask: fn(),
    onTaskClick: fn()
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    tasks: sampleTasks.filter(t => t.status === "open" || t.status === "in_progress"),
    onCompleteTask: fn(),
    onCancelTask: fn(),
    onTaskClick: fn()
  }
}`,...h.parameters?.docs?.source}}},g=[`Default`,`Empty`,`Loading`,`OpenOnly`,`WithOverdue`]}))();export{d as Default,f as Empty,p as Loading,m as OpenOnly,h as WithOverdue,g as __namedExportsOrder,u as default};