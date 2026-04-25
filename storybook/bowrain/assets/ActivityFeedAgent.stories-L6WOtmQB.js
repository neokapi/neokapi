import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./ActivityFeed-CpKX03-x.js";var i,a,o,s,c,l,u,d,f,p;e((()=>{n(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o={title:`Pages/Activity/ActivityFeedAgent`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{maxWidth:480,padding:16},children:(0,i.jsx)(e,{})})]},s=Date.now(),c=[{id:`1`,workspace_id:`ws-1`,actor_id:`u1`,actor_name:`Alice`,type:`agent.conversation.created`,summary:``,data:{title:`Translate French files`},created_at:new Date(s-6e4).toISOString()},{id:`2`,workspace_id:`ws-1`,actor_id:`u1`,actor_name:`Alice`,type:`agent.message.sent`,summary:``,data:{blocks_count:`45`},created_at:new Date(s-12e4).toISOString()},{id:`3`,workspace_id:`ws-1`,actor_id:`bravo`,actor_name:`@bravo`,type:`agent.tool.executed`,summary:``,data:{tool:`run_flow`,duration:`2.3s`},created_at:new Date(s-18e4).toISOString()},{id:`4`,workspace_id:`ws-1`,actor_id:`u1`,actor_name:`Alice`,type:`agent.tool.approved`,summary:``,data:{tool:`connector_push`},created_at:new Date(s-24e4).toISOString()},{id:`5`,workspace_id:`ws-1`,actor_id:`u1`,actor_name:`Bob`,type:`agent.tool.denied`,summary:``,data:{tool:`execute_script`},created_at:new Date(s-3e5).toISOString()},{id:`6`,workspace_id:`ws-1`,actor_id:`bravo`,actor_name:`@bravo`,type:`agent.code.executed`,summary:``,data:{language:`python`,exit_code:`0`},created_at:new Date(s-36e4).toISOString()},{id:`7`,workspace_id:`ws-1`,actor_id:`bravo`,actor_name:`@bravo`,type:`agent.code.executed`,summary:``,data:{language:`bash`,exit_code:`1`},created_at:new Date(s-42e4).toISOString()}],l=[{id:`m1`,workspace_id:`ws-1`,actor_id:`u1`,actor_name:`Alice`,type:`project.created`,summary:`created project Mobile App`,data:{name:`Mobile App`},project_id:`p1`,created_at:new Date(s-6e4).toISOString()},...c.slice(0,3),{id:`m2`,workspace_id:`ws-1`,actor_id:`u2`,actor_name:`Bob`,type:`flow.completed`,summary:`completed pseudo-translate flow`,created_at:new Date(s-5e5).toISOString()}],u={args:{activities:c,onActivityClick:a()}},d={args:{activities:l,hasMore:!0,onLoadMore:a(),onActivityClick:a()}},f={args:{activities:[]}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    activities: agentActivities,
    onActivityClick: fn()
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    activities: mixedActivities,
    hasMore: true,
    onLoadMore: fn(),
    onActivityClick: fn()
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    activities: []
  }
}`,...f.parameters?.docs?.source}}},p=[`AgentEventsOnly`,`MixedWithAgentEvents`,`Empty`]}))();export{u as AgentEventsOnly,f as Empty,d as MixedWithAgentEvents,p as __namedExportsOrder,o as default};