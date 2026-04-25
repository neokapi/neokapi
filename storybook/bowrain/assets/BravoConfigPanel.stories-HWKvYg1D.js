import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{L as n,R as r}from"./iframe-BJVhYAGS.js";import{i,n as a,t as o}from"./fixtures-DC2kBE5i.js";var s,c,l,u,d,f,p,m,h,g,_;e((()=>{r(),o(),s=t(),{fn:c}=__STORYBOOK_MODULE_TEST__,l={title:`Bravo/BravoConfigPanel`,component:n,tags:[`autodocs`],decorators:[e=>(0,s.jsx)(`div`,{style:{maxWidth:480,border:`1px solid #ddd`,borderRadius:8,padding:16},children:(0,s.jsx)(e,{})})]},u={args:{config:a,tools:i,onSave:c()}},d={args:{config:{...a,enabled:!1},tools:i,onSave:c()}},f={args:{config:{...a,denied_tools:i.map(e=>e.name),require_approval:[]},tools:i,onSave:c()}},p={args:{config:{...a,denied_tools:[],require_approval:i.map(e=>e.name)},tools:i,onSave:c()}},m={args:{config:{...a,denied_tools:[`execute_script`],require_approval:[`connector_push`,`connector_pull`,`create_version`]},tools:[...i,{name:`create_version`,require_approval:!0},{name:`list_streams`,require_approval:!1},{name:`diff_streams`,require_approval:!1},{name:`merge_stream`,require_approval:!0},{name:`update_block`,require_approval:!1},{name:`term_search`,require_approval:!1},{name:`term_add`,require_approval:!1},{name:`tm_import`,require_approval:!1},{name:`execute_script`,require_approval:!1},{name:`check_vocabulary`,require_approval:!1},{name:`list_profiles`,require_approval:!1}],onSave:c()}},h={args:{config:a,tools:i,onSave:c(),saving:!0}},g={args:{config:a,tools:[],onSave:c()}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    config: sampleConfig,
    tools: sampleTools,
    onSave: fn()
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    config: {
      ...sampleConfig,
      enabled: false
    },
    tools: sampleTools,
    onSave: fn()
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    config: {
      ...sampleConfig,
      denied_tools: sampleTools.map(t => t.name),
      require_approval: []
    },
    tools: sampleTools,
    onSave: fn()
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    config: {
      ...sampleConfig,
      denied_tools: [],
      require_approval: sampleTools.map(t => t.name)
    },
    tools: sampleTools,
    onSave: fn()
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    config: {
      ...sampleConfig,
      denied_tools: ["execute_script"],
      require_approval: ["connector_push", "connector_pull", "create_version"]
    },
    tools: [...sampleTools, {
      name: "create_version",
      require_approval: true
    }, {
      name: "list_streams",
      require_approval: false
    }, {
      name: "diff_streams",
      require_approval: false
    }, {
      name: "merge_stream",
      require_approval: true
    }, {
      name: "update_block",
      require_approval: false
    }, {
      name: "term_search",
      require_approval: false
    }, {
      name: "term_add",
      require_approval: false
    }, {
      name: "tm_import",
      require_approval: false
    }, {
      name: "execute_script",
      require_approval: false
    }, {
      name: "check_vocabulary",
      require_approval: false
    }, {
      name: "list_profiles",
      require_approval: false
    }],
    onSave: fn()
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    config: sampleConfig,
    tools: sampleTools,
    onSave: fn(),
    saving: true
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    config: sampleConfig,
    tools: [],
    onSave: fn()
  }
}`,...g.parameters?.docs?.source}}},_=[`Default`,`Disabled`,`AllToolsDenied`,`AllToolsRequireApproval`,`ManyTools`,`Saving`,`NoTools`]}))();export{f as AllToolsDenied,p as AllToolsRequireApproval,u as Default,d as Disabled,m as ManyTools,g as NoTools,h as Saving,_ as __namedExportsOrder,l as default};