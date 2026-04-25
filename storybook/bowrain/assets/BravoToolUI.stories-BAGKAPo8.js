import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{H as n,V as r}from"./iframe-Cr-Gz2Kj.js";var i,a,o,s,c,l,u,d,f;e((()=>{n(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o={title:`Bravo/BravoToolUI`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{maxWidth:480,padding:16},children:(0,i.jsx)(e,{})})]},s={args:{toolName:`translate_block`,toolCallId:`call_001`,args:{block_id:`seg-42`,source:`Hello, world!`,target_locale:`fr-FR`},result:{translation:`Bonjour le monde !`},status:{type:`complete`}}},c={args:{toolName:`quality_check`,toolCallId:`call_002`,args:{file:`messages.json`,locale:`de-DE`},status:{type:`running`}}},l={args:{toolName:`push_connector`,toolCallId:`call_003`,args:{connector:`github`,branch:`l10n/main`},result:{error:`Authentication failed: token expired`},isError:!0,status:{type:`error`}}},u={args:{toolName:`delete_translations`,toolCallId:`call_004`,args:{locale:`ja-JP`,count:47},status:{type:`requires-action`},addResult:a()}},d={args:{toolName:`batch_translate`,toolCallId:`call_005`,args:{source_locale:`en-US`,target_locales:[`fr-FR`,`de-DE`,`es-ES`]},status:{type:`pending`}}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    toolName: "translate_block",
    toolCallId: "call_001",
    args: {
      block_id: "seg-42",
      source: "Hello, world!",
      target_locale: "fr-FR"
    },
    result: {
      translation: "Bonjour le monde !"
    },
    status: {
      type: "complete"
    }
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    toolName: "quality_check",
    toolCallId: "call_002",
    args: {
      file: "messages.json",
      locale: "de-DE"
    },
    status: {
      type: "running"
    }
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    toolName: "push_connector",
    toolCallId: "call_003",
    args: {
      connector: "github",
      branch: "l10n/main"
    },
    result: {
      error: "Authentication failed: token expired"
    },
    isError: true,
    status: {
      type: "error"
    }
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    toolName: "delete_translations",
    toolCallId: "call_004",
    args: {
      locale: "ja-JP",
      count: 47
    },
    status: {
      type: "requires-action"
    },
    addResult: fn()
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    toolName: "batch_translate",
    toolCallId: "call_005",
    args: {
      source_locale: "en-US",
      target_locales: ["fr-FR", "de-DE", "es-ES"]
    },
    status: {
      type: "pending"
    }
  }
}`,...d.parameters?.docs?.source}}},f=[`Completed`,`Running`,`Error`,`RequiresApproval`,`Pending`]}))();export{s as Completed,l as Error,d as Pending,u as RequiresApproval,c as Running,f as __namedExportsOrder,o as default};