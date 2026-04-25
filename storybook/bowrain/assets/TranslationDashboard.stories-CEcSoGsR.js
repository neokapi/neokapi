import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./TranslationDashboard-Bd7nBLe-.js";import{N as i,f as a,l as o}from"./fixtures-D8OXoGLB.js";import{n as s,r as c}from"./decorators-BptasVUO.js";var l,u,d,f,p,m,h,g,_;e((()=>{n(),s(),o(),l=t(),u={title:`Pages/Translation/TranslationDashboard`,component:r,tags:[`autodocs`],decorators:[c,e=>(0,l.jsx)(`div`,{style:{maxWidth:1100,padding:24},children:(0,l.jsx)(e,{})})]},d={args:{stats:null,projectName:`New Project`}},f={args:{stats:i,projectName:`Demo App`}},p={args:{stats:a,projectName:`Marketing Platform`}},m={args:{stats:i}},h={args:{stats:{...i,locale_stats:i.locale_stats.map(e=>({...e,translated_blocks:e.total_blocks,translated_words:e.total_words,percentage:100}))},projectName:`Completed Project`}},g={args:{stats:{...i,locale_stats:[i.locale_stats[0]],item_stats:i.item_stats.map(e=>({...e,locales:[e.locales[0]]})),collection_stats:i.collection_stats.map(e=>({...e,locales:[e.locales[0]]}))},projectName:`French Only`}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    stats: null,
    projectName: "New Project"
  }
}`,...d.parameters?.docs?.source},description:{story:`No data yet — shows the empty onboarding state.`,...d.parameters?.docs?.description}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    stats: sampleDashboardStats,
    projectName: "Demo App"
  }
}`,...f.parameters?.docs?.source},description:{story:`Typical project with 3 locales, 3 files, and 2 collections.`,...f.parameters?.docs?.description}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    stats: largeDashboardStats,
    projectName: "Marketing Platform"
  }
}`,...p.parameters?.docs?.source},description:{story:`Large project with 6 locales and 4 files across 2 collections.`,...p.parameters?.docs?.description}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    stats: sampleDashboardStats
  }
}`,...m.parameters?.docs?.source},description:{story:`Dashboard without a project name in the header.`,...m.parameters?.docs?.description}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    stats: {
      ...sampleDashboardStats,
      locale_stats: sampleDashboardStats.locale_stats.map(l => ({
        ...l,
        translated_blocks: l.total_blocks,
        translated_words: l.total_words,
        percentage: 100
      }))
    },
    projectName: "Completed Project"
  }
}`,...h.parameters?.docs?.source},description:{story:`Fully translated project — all locales at 100%.`,...h.parameters?.docs?.description}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    stats: {
      ...sampleDashboardStats,
      locale_stats: [sampleDashboardStats.locale_stats[0]],
      item_stats: sampleDashboardStats.item_stats.map(item => ({
        ...item,
        locales: [item.locales[0]]
      })),
      collection_stats: sampleDashboardStats.collection_stats.map(coll => ({
        ...coll,
        locales: [coll.locales[0]]
      }))
    },
    projectName: "French Only"
  }
}`,...g.parameters?.docs?.source},description:{story:`Single locale — minimal chart layout.`,...g.parameters?.docs?.description}}},_=[`Empty`,`Default`,`LargeProject`,`NoProjectName`,`FullyTranslated`,`SingleLocale`]}))();export{f as Default,d as Empty,h as FullyTranslated,p as LargeProject,m as NoProjectName,g as SingleLocale,_ as __namedExportsOrder,u as default};