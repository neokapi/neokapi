import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./ActivityFeed-CpKX03-x.js";var i,a,o,s,c,l,u,d,f,p,m,h;e((()=>{n(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o=new Date,s=e=>new Date(o.getTime()-e*6e4).toISOString(),c=[{id:`act-1`,workspace_id:`ws-1`,project_id:`proj-1`,actor_id:`user-1`,actor_name:`Alice Chen`,type:`block.translated`,entity_type:`block`,entity_id:`blk-001`,summary:`updated block greeting.title`,data:{name:`Marketing Website`},created_at:s(2)},{id:`act-2`,workspace_id:`ws-1`,project_id:`proj-1`,actor_id:`user-2`,actor_name:`Bob Martinez`,type:`stream.created`,entity_type:`stream`,entity_id:`feature/translations`,summary:`created stream feature/translations`,created_at:s(15)},{id:`act-3`,workspace_id:`ws-1`,project_id:`proj-1`,actor_id:`system`,actor_name:`System`,type:`flow.completed`,entity_type:`flow`,summary:`flow completed`,created_at:s(30)},{id:`act-4`,workspace_id:`ws-1`,project_id:`proj-2`,actor_id:`user-1`,actor_name:`Alice Chen`,type:`project.created`,entity_type:`project`,entity_id:`proj-2`,summary:`created project Mobile App`,data:{name:`Mobile App`},created_at:s(120)},{id:`act-5`,workspace_id:`ws-1`,project_id:`proj-1`,actor_id:`system`,actor_name:`System`,type:`gate.failed`,entity_type:`gate`,summary:`quality gate failed`,created_at:s(180)},{id:`act-6`,workspace_id:`ws-1`,project_id:`proj-1`,actor_id:`system`,actor_name:`System`,type:`brand.drift`,entity_type:`brand`,summary:`brand voice drift detected`,created_at:s(300)},{id:`act-7`,workspace_id:`ws-1`,project_id:`proj-1`,actor_id:`user-3`,actor_name:`Carol Wang`,type:`item.pushed`,entity_type:`item`,summary:`pushed locales/en.json, locales/fr.json`,created_at:s(500)},{id:`act-8`,workspace_id:`ws-1`,project_id:`proj-1`,actor_id:`system`,actor_name:`System`,type:`extraction.done`,entity_type:`extraction`,summary:`extraction completed`,created_at:s(1440)},{id:`act-9`,workspace_id:`ws-1`,project_id:`proj-1`,actor_id:`user-2`,actor_name:`Bob Martinez`,type:`stream.merged`,entity_type:`stream`,entity_id:`feature/translations`,summary:`merged stream feature/translations`,created_at:s(2880)}],l={title:`Pages/Activity/ActivityFeed`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{maxWidth:480,padding:24},children:(0,i.jsx)(e,{})})]},u={args:{activities:c,hasMore:!0,onLoadMore:a(),onActivityClick:a()}},d={args:{activities:[],hasMore:!1}},f={args:{activities:[],loading:!0}},p={args:{activities:c.slice(0,3),hasMore:!1,onActivityClick:a()}},m={args:{activities:c.filter(e=>e.type.includes(`failed`)||e.type.includes(`drift`)),hasMore:!1,onActivityClick:a()}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    activities: sampleActivities,
    hasMore: true,
    onLoadMore: fn(),
    onActivityClick: fn()
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    activities: [],
    hasMore: false
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    activities: [],
    loading: true
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    activities: sampleActivities.slice(0, 3),
    hasMore: false,
    onActivityClick: fn()
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    activities: sampleActivities.filter(a => a.type.includes("failed") || a.type.includes("drift")),
    hasMore: false,
    onActivityClick: fn()
  }
}`,...m.parameters?.docs?.source}}},h=[`Default`,`Empty`,`Loading`,`NoMore`,`FailureEvents`]}))();export{u as Default,d as Empty,m as FailureEvents,f as Loading,p as NoMore,h as __namedExportsOrder,l as default};