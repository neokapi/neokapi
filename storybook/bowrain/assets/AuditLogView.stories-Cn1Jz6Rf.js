import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./AuditLogView-g0fvuCX7.js";import{n as i,r as a}from"./decorators-BptasVUO.js";var o,s,c,l,u,d,f,p,m,h,g;e((()=>{n(),i(),o=t(),{fn:s}=__STORYBOOK_MODULE_TEST__,c=new Date,l=e=>new Date(c.getTime()-e*6e4).toISOString(),u=[{id:`proj-1`,name:`Marketing Website`,default_source_language:`en-US`,target_languages:[`fr-FR`,`de-DE`],created_at:l(1e4),modified_at:l(5)},{id:`proj-2`,name:`Mobile App`,default_source_language:`en`,target_languages:[`ja`,`ko`],created_at:l(2e4),modified_at:l(120)}],d=[{id:1,project_id:`proj-1`,event_type:`project.created`,actor:`alice@example.com`,source:`web`,data:JSON.stringify({name:`Marketing Website`}),created_at:l(5)},{id:2,project_id:`proj-1`,event_type:`item.created`,actor:`alice@example.com`,source:`web`,data:JSON.stringify({item_name:`locales/en.json`,stream:`main`,format:`json`}),created_at:l(4)},{id:3,project_id:`proj-1`,event_type:`block.updated`,actor:`bob@example.com`,source:`editor`,data:JSON.stringify({block_id:`abc12345-def6-7890`,item_name:`locales/en.json`}),created_at:l(3)},{id:4,project_id:`proj-1`,event_type:`stream.created`,actor:`alice@example.com`,source:`web`,data:JSON.stringify({stream:`feature/translations`,parent:`main`}),created_at:l(2)},{id:5,project_id:`proj-2`,event_type:`connector.push.completed`,actor:`ci-bot`,source:`sync`,data:JSON.stringify({items:`src/i18n/en.json,src/i18n/common.json`,push_id:`push-001`}),created_at:l(90)}],f={title:`Pages/Audit/AuditLogView`,component:r,tags:[`autodocs`],decorators:[a,e=>(0,o.jsx)(`div`,{style:{maxWidth:960,padding:24},children:(0,o.jsx)(e,{})})]},p={args:{entries:d,projects:u,hasMore:!0,onLoadMore:s(),onFiltersChange:s(),onSearchChange:s(),activeFilters:[],activeSearch:``}},m={args:{entries:[],projects:u,hasMore:!1,onFiltersChange:s(),onSearchChange:s()}},h={args:{entries:d.filter(e=>e.event_type.startsWith(`project`)),projects:u,hasMore:!1,activeFilters:[{key:`type`,value:`project`}],activeSearch:``,onFiltersChange:s(),onSearchChange:s()}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    entries: sampleEntries,
    projects: sampleProjects,
    hasMore: true,
    onLoadMore: fn(),
    onFiltersChange: fn(),
    onSearchChange: fn(),
    activeFilters: [],
    activeSearch: ""
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    entries: [],
    projects: sampleProjects,
    hasMore: false,
    onFiltersChange: fn(),
    onSearchChange: fn()
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    entries: sampleEntries.filter(e => e.event_type.startsWith("project")),
    projects: sampleProjects,
    hasMore: false,
    activeFilters: [{
      key: "type",
      value: "project"
    }],
    activeSearch: "",
    onFiltersChange: fn(),
    onSearchChange: fn()
  }
}`,...h.parameters?.docs?.source}}},g=[`Default`,`Empty`,`WithFilters`]}))();export{p as Default,m as Empty,h as WithFilters,g as __namedExportsOrder,f as default};