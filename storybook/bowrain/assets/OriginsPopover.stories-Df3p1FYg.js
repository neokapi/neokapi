import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./OriginsPopover-BE4lE9r2.js";var i,a,o,s,c,l,u,d,f,p,m,h,g,_;e((()=>{n(),i=t(),a=new Date().toISOString(),o=new Date(Date.now()-1440*60*1e3).toISOString(),s=new Date(Date.now()-10080*60*1e3).toISOString(),c=[{source:`file`,key:`apps/web/locales/en-US.json:errors.notFound`,reference:`commit:abc123ef`,added_at:o,added_by:`tmx-import`}],l=[{source:`file`,key:`apps/web/locales/en-US.json:errors.notFound`,reference:`commit:abc123ef`,added_at:s,added_by:`tmx-import`},{source:`file`,key:`apps/mobile/strings.xml:error_not_found`,added_at:o,added_by:`tmx-import`},{source:`tool`,key:`ai-translate`,reference:`job-42`,added_at:a,added_by:`kapi`},{source:`user`,added_at:a,added_by:`alice@example.com`}],u={title:`Resource Browser/OriginsPopover`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{padding:40},children:(0,i.jsx)(e,{})})],parameters:{docs:{description:{component:`Provenance popover for TM entries. Shows a count badge of origins (file, tool, import, user) and an optional translator note. Click to expand.`}}}},d={args:{origins:c}},f={args:{origins:l}},p={args:{origins:c,note:`This is shown in the welcome screen — keep the tone friendly and concise.`}},m={args:{origins:[],note:`Translator note without any recorded origin.`}},h={args:{origins:[]},parameters:{docs:{description:{story:`When there are no origins and no note, the component renders nothing.`}}}},g={args:{origins:[{source:`import`,key:`acme-glossary.tmx`,session_id:`sess-1`,added_at:s,added_by:`tmx-import`}]}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    origins: singleFileOrigin
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    origins: multipleOrigins
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    origins: singleFileOrigin,
    note: "This is shown in the welcome screen — keep the tone friendly and concise."
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    origins: [],
    note: "Translator note without any recorded origin."
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    origins: []
  },
  parameters: {
    docs: {
      description: {
        story: "When there are no origins and no note, the component renders nothing."
      }
    }
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    origins: [{
      source: "import",
      key: "acme-glossary.tmx",
      session_id: "sess-1",
      added_at: lastWeek,
      added_by: "tmx-import"
    }]
  }
}`,...g.parameters?.docs?.source},description:{story:`Origin with a session_id shown inline (no async fetch).`,...g.parameters?.docs?.description}}},_=[`SingleOrigin`,`MultipleOrigins`,`WithNote`,`NoteOnly`,`Empty`,`WithSessionOrigin`]}))();export{h as Empty,f as MultipleOrigins,m as NoteOnly,d as SingleOrigin,p as WithNote,g as WithSessionOrigin,_ as __namedExportsOrder,u as default};