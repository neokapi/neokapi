import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{a as n,i as r}from"./HighlightedSource-BHFzDJ_q.js";var i,a,o,s,c,l,u,d,f;e((()=>{n(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o={key:`ent-1`,text:`John Smith`,type:`entity:person`,start:0,end:10,dnt:!1},s={key:`ent-2`,text:`Acme Corp`,type:`entity:organization`,start:22,end:31,dnt:!0,source:`manual`},c={title:`Editor/Entities/EntityPopover`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{position:`relative`,padding:80},children:(0,i.jsx)(e,{})})]},l={args:{entity:o,onClose:a(),onUpdate:a(),onDelete:a(),onPromote:a()}},u={args:{entity:s,onClose:a(),onUpdate:a(),onDelete:a()}},d={args:{entity:o,onClose:a()}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    entity: sampleEntity,
    onClose: fn(),
    onUpdate: fn(),
    onDelete: fn(),
    onPromote: fn()
  }
}`,...l.parameters?.docs?.source},description:{story:`View/edit an existing entity with all actions.`,...l.parameters?.docs?.description}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    entity: dntEntity,
    onClose: fn(),
    onUpdate: fn(),
    onDelete: fn()
  }
}`,...u.parameters?.docs?.source},description:{story:`Do-not-translate entity.`,...u.parameters?.docs?.description}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    entity: sampleEntity,
    onClose: fn()
  }
}`,...d.parameters?.docs?.source},description:{story:`Read-only — no update/delete actions.`,...d.parameters?.docs?.description}}},f=[`WithAllActions`,`DNTEntity`,`ReadOnly`]}))();export{u as DNTEntity,d as ReadOnly,l as WithAllActions,f as __namedExportsOrder,c as default};