import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./TermCreationPopover-ygx7HoZG.js";var i,a,o,s,c,l,u,d;e((()=>{n(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o={title:`Editor/Terminology/TermCreationPopover`,component:r,tags:[`autodocs`],args:{onSubmit:a(),onClose:a(),sourceLocale:`en-US`,targetLocale:`fr-FR`},decorators:[e=>(0,i.jsx)(`div`,{style:{padding:100,minHeight:400},children:(0,i.jsx)(e,{})})]},s={args:{open:!0,selectedText:`localization`}},c={args:{open:!1,selectedText:``}},l={args:{open:!0,selectedText:`translation memory`}},u={args:{open:!0,selectedText:`quality assurance`,targetLocale:`ja-JP`}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    selectedText: "localization"
  }
}`,...s.parameters?.docs?.source},description:{story:`Popover open with pre-selected source text`,...s.parameters?.docs?.description}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    open: false,
    selectedText: ""
  }
}`,...c.parameters?.docs?.source},description:{story:`Popover closed`,...c.parameters?.docs?.description}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    selectedText: "translation memory"
  }
}`,...l.parameters?.docs?.source},description:{story:`Multi-word source text selection`,...l.parameters?.docs?.description}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    selectedText: "quality assurance",
    targetLocale: "ja-JP"
  }
}`,...u.parameters?.docs?.source},description:{story:`Japanese target locale`,...u.parameters?.docs?.description}}},d=[`Open`,`Closed`,`MultiWordSelection`,`JapaneseTarget`]}))();export{c as Closed,u as JapaneseTarget,l as MultiWordSelection,s as Open,d as __namedExportsOrder,o as default};