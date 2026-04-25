import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./ProjectDashboard-CYYQhI2r.js";import{F as i,l as a}from"./fixtures-D8OXoGLB.js";import{n as o,r as s}from"./decorators-BptasVUO.js";var c,l,u,d,f,p,m,h,g,_,v,y,b;e((()=>{n(),o(),a(),c=t(),{fn:l}=__STORYBOOK_MODULE_TEST__,u={title:`Pages/Workspace/ProjectDashboard`,component:r,tags:[`autodocs`],decorators:[s,e=>(0,c.jsx)(`div`,{style:{maxWidth:960,padding:24},children:(0,c.jsx)(e,{})})]},d={...i,id:`proj-2`,name:`Marketing Website`,default_source_language:`en-US`,target_languages:[`fr-FR`,`de-DE`,`ja-JP`,`es-ES`,`zh-CN`],items:[{id:`itm-land`,name:`landing.html`,format:`html`,type:`file`,size:12e3,block_count:48,word_count:1320},{id:`itm-abt`,name:`about.html`,format:`html`,type:`file`,size:8e3,block_count:32,word_count:910},{id:`itm-prc`,name:`pricing.json`,format:`json`,type:`file`,size:3e3,block_count:20,word_count:240}],streams:[{name:`main`,parent:``,base_cursor:0,archived:!1,visibility:`public`,description:``,created_at:`2025-12-01T10:00:00Z`,created_by:`user-1`},{name:`q1-campaign`,parent:`main`,base_cursor:5,archived:!1,visibility:`public`,description:`Q1 marketing campaign`,created_at:`2026-02-01T10:00:00Z`,created_by:`user-1`}],created_at:`2025-12-01T10:00:00Z`,modified_at:`2026-03-13T09:15:00Z`},f={...i,id:`proj-3`,name:`Mobile App Strings`,default_source_language:`en`,target_languages:[`fr`,`de`],items:[{id:`itm-str`,name:`strings.xml`,format:`xml`,type:`file`,size:3200,block_count:120,word_count:650}],created_at:`2026-01-15T10:00:00Z`,modified_at:`2026-03-12T16:45:00Z`},p={...i,id:`proj-4`,name:`API Documentation`,default_source_language:`en`,target_languages:[`ja`,`ko`,`zh-CN`,`pt-BR`],items:[{id:`itm-gs`,name:`getting-started.md`,format:`md`,type:`file`,size:15e3,block_count:85,word_count:2400},{id:`itm-api`,name:`api-reference.md`,format:`md`,type:`file`,size:28e3,block_count:200,word_count:5800},{id:`itm-cl`,name:`changelog.md`,format:`md`,type:`file`,size:4e3,block_count:30,word_count:450}],created_at:`2025-10-20T10:00:00Z`,modified_at:`2026-03-14T11:30:00Z`},m={...i,id:`proj-5`,name:`WordPress Blog`,default_source_language:`en-US`,target_languages:[`es-ES`,`fr-FR`,`pt-BR`],items:[{id:`itm-post`,name:`posts.xliff`,format:`xliff`,type:`file`,size:45e3,block_count:310,word_count:8200},{id:`itm-page`,name:`pages.xliff`,format:`xliff`,type:`file`,size:12e3,block_count:60,word_count:1500}],created_at:`2026-02-01T10:00:00Z`,modified_at:`2026-03-10T08:20:00Z`},h={args:{projects:[],onCreateProject:l(),onOpenProject:l(),onCreateSampleProject:l(),workspaceName:`My Workspace`}},g={args:{projects:[],onCreateProject:l(),onOpenProject:l(),workspaceName:`Acme Corp`}},_={args:{projects:[i],onCreateProject:l(),onOpenProject:l(),workspaceName:`My Workspace`}},v={args:{projects:[i,d,f,p,m],onCreateProject:l(),onOpenProject:l(),workspaceName:`Acme Corp`}},y={args:{projects:[i,d,f,p,m,{...i,id:`proj-6`,name:`iOS Storyboard`,target_languages:[`ja`,`ko`],modified_at:`2026-03-08T14:00:00Z`},{...f,id:`proj-7`,name:`Android Resources`,modified_at:`2026-03-05T10:00:00Z`},{...p,id:`proj-8`,name:`Help Center`,target_languages:[`fr`,`de`,`es`],modified_at:`2026-03-01T08:00:00Z`}],onCreateProject:l(),onOpenProject:l(),workspaceName:`Globalize Inc`}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    projects: [],
    onCreateProject: fn(),
    onOpenProject: fn(),
    onCreateSampleProject: fn(),
    workspaceName: "My Workspace"
  }
}`,...h.parameters?.docs?.source},description:{story:`Empty workspace — shows the onboarding experience with getting-started pathways.`,...h.parameters?.docs?.description}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    projects: [],
    onCreateProject: fn(),
    onOpenProject: fn(),
    workspaceName: "Acme Corp"
  }
}`,...g.parameters?.docs?.source},description:{story:`Empty workspace without sample project option.`,...g.parameters?.docs?.description}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  args: {
    projects: [sampleProject],
    onCreateProject: fn(),
    onOpenProject: fn(),
    workspaceName: "My Workspace"
  }
}`,..._.parameters?.docs?.source},description:{story:`Single project — minimal dashboard.`,..._.parameters?.docs?.description}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  args: {
    projects: [sampleProject, marketingProject, mobileProject, docsProject, wpProject],
    onCreateProject: fn(),
    onOpenProject: fn(),
    workspaceName: "Acme Corp"
  }
}`,...v.parameters?.docs?.source},description:{story:`Multiple projects with varied sizes and language counts.`,...v.parameters?.docs?.description}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  args: {
    projects: [sampleProject, marketingProject, mobileProject, docsProject, wpProject, {
      ...sampleProject,
      id: "proj-6",
      name: "iOS Storyboard",
      target_languages: ["ja", "ko"],
      modified_at: "2026-03-08T14:00:00Z"
    }, {
      ...mobileProject,
      id: "proj-7",
      name: "Android Resources",
      modified_at: "2026-03-05T10:00:00Z"
    }, {
      ...docsProject,
      id: "proj-8",
      name: "Help Center",
      target_languages: ["fr", "de", "es"],
      modified_at: "2026-03-01T08:00:00Z"
    }],
    onCreateProject: fn(),
    onOpenProject: fn(),
    workspaceName: "Globalize Inc"
  }
}`,...y.parameters?.docs?.source},description:{story:`Many projects to demonstrate grid scaling.`,...y.parameters?.docs?.description}}},b=[`Empty`,`EmptyNoSample`,`SingleProject`,`WithProjects`,`ManyProjects`]}))();export{h as Empty,g as EmptyNoSample,y as ManyProjects,_ as SingleProject,v as WithProjects,b as __namedExportsOrder,u as default};