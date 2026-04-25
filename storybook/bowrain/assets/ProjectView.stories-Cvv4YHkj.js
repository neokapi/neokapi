import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./ProjectView-DIupr007.js";import{F as i,l as a}from"./fixtures-D8OXoGLB.js";import{n as o,r as s}from"./decorators-BptasVUO.js";var c,l,u,d,f,p,m,h,g;e((()=>{n(),o(),a(),c=t(),{fn:l}=__STORYBOOK_MODULE_TEST__,u={title:`Pages/Workspace/ProjectView`,component:r,tags:[`autodocs`],decorators:[s,e=>(0,c.jsx)(`div`,{style:{maxWidth:960,padding:24},children:(0,c.jsx)(e,{})})]},d={args:{project:i,onBack:l(),onOpenFile:l(),onUploadFiles:l(),onRemoveFile:l(),onOpenTM:l(),onOpenTerms:l(),onEditProject:l(),onArchiveProject:l(),onManageMembers:l(),onOpenSettings:l(),onCreateCollection:l(),onEditCollection:l(),onDeleteCollection:l(),onUploadToCollection:l(),onCreateStream:l(),onMergeStream:l(),onDiffStream:l(),onDeleteStream:l()}},f={args:{...d.args,project:{...i,dashboard_visibility:`public`}}},p={args:{project:i,onBack:l(),onOpenFile:l(),onUploadFiles:l(),onRemoveFile:l(),onOpenTM:l(),onOpenTerms:l(),serverMode:{serverURL:`https://bowrain.example.com`,workspaceSlug:`demo`},onCreateCollection:l(),onEditCollection:l(),onDeleteCollection:l(),onCreateStream:l()}},m={args:{project:{...i,items:[],collections:i.collections?.map(e=>({...e,item_count:0}))},onBack:l(),onOpenFile:l(),onUploadFiles:l(),onRemoveFile:l(),onCreateCollection:l(),onCreateStream:l()}},h={args:{project:{...i,collections:i.collections?.slice(0,1)},onBack:l(),onOpenFile:l(),onUploadFiles:l(),onRemoveFile:l()}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    project: sampleProject,
    onBack: fn(),
    onOpenFile: fn(),
    onUploadFiles: fn(),
    onRemoveFile: fn(),
    onOpenTM: fn(),
    onOpenTerms: fn(),
    onEditProject: fn(),
    onArchiveProject: fn(),
    onManageMembers: fn(),
    onOpenSettings: fn(),
    onCreateCollection: fn(),
    onEditCollection: fn(),
    onDeleteCollection: fn(),
    onUploadToCollection: fn(),
    onCreateStream: fn(),
    onMergeStream: fn(),
    onDiffStream: fn(),
    onDeleteStream: fn()
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    ...Default.args,
    project: {
      ...sampleProject,
      dashboard_visibility: "public"
    }
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    project: sampleProject,
    onBack: fn(),
    onOpenFile: fn(),
    onUploadFiles: fn(),
    onRemoveFile: fn(),
    onOpenTM: fn(),
    onOpenTerms: fn(),
    serverMode: {
      serverURL: "https://bowrain.example.com",
      workspaceSlug: "demo"
    },
    onCreateCollection: fn(),
    onEditCollection: fn(),
    onDeleteCollection: fn(),
    onCreateStream: fn()
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    project: {
      ...sampleProject,
      items: [],
      collections: sampleProject.collections?.map(c => ({
        ...c,
        item_count: 0
      }))
    },
    onBack: fn(),
    onOpenFile: fn(),
    onUploadFiles: fn(),
    onRemoveFile: fn(),
    onCreateCollection: fn(),
    onCreateStream: fn()
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    project: {
      ...sampleProject,
      collections: sampleProject.collections?.slice(0, 1)
    },
    onBack: fn(),
    onOpenFile: fn(),
    onUploadFiles: fn(),
    onRemoveFile: fn()
  }
}`,...h.parameters?.docs?.source}}},g=[`Default`,`PublicOnPulse`,`WithServerMode`,`EmptyProject`,`SingleCollection`]}))();export{d as Default,m as EmptyProject,f as PublicOnPulse,h as SingleCollection,p as WithServerMode,g as __namedExportsOrder,u as default};