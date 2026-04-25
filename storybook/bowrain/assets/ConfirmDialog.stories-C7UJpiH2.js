import{n as e}from"./chunk-DnJy8xQt.js";import{n as t,t as n}from"./ConfirmDialog-DvB0Ugp6.js";var r,i,a,o,s,c,l;e((()=>{t(),{fn:r}=__STORYBOOK_MODULE_TEST__,i={title:`Foundations/ConfirmDialog`,component:n,tags:[`autodocs`]},a={args:{open:!0,onOpenChange:r(),title:`Confirm Action`,description:`Are you sure you want to proceed? This action cannot be undone.`,onConfirm:r()}},o={args:{open:!0,onOpenChange:r(),title:`Delete Project`,description:`This will permanently delete the project and all its translations.`,confirmLabel:`Delete`,variant:`destructive`,onConfirm:r()}},s={args:{open:!0,onOpenChange:r(),title:`Delete Project`,description:`This will permanently delete the project.`,confirmLabel:`Delete`,variant:`destructive`,loading:!0,onConfirm:r()}},c={args:{open:!0,onOpenChange:r(),title:`Archive Stream`,description:`Archive this stream? It can be restored later from the bin.`,confirmLabel:`Yes, archive it`,cancelLabel:`Keep it`,onConfirm:r()}},a.parameters={...a.parameters,docs:{...a.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: fn(),
    title: "Confirm Action",
    description: "Are you sure you want to proceed? This action cannot be undone.",
    onConfirm: fn()
  }
}`,...a.parameters?.docs?.source}}},o.parameters={...o.parameters,docs:{...o.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: fn(),
    title: "Delete Project",
    description: "This will permanently delete the project and all its translations.",
    confirmLabel: "Delete",
    variant: "destructive",
    onConfirm: fn()
  }
}`,...o.parameters?.docs?.source}}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: fn(),
    title: "Delete Project",
    description: "This will permanently delete the project.",
    confirmLabel: "Delete",
    variant: "destructive",
    loading: true,
    onConfirm: fn()
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    open: true,
    onOpenChange: fn(),
    title: "Archive Stream",
    description: "Archive this stream? It can be restored later from the bin.",
    confirmLabel: "Yes, archive it",
    cancelLabel: "Keep it",
    onConfirm: fn()
  }
}`,...c.parameters?.docs?.source}}},l=[`Default`,`Destructive`,`Loading`,`CustomLabels`]}))();export{c as CustomLabels,a as Default,o as Destructive,s as Loading,l as __namedExportsOrder,i as default};