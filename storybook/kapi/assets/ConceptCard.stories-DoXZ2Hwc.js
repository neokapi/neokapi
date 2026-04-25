import{n as e}from"./chunk-DnJy8xQt.js";import{n as t,t as n}from"./ConceptCard-IcBwHjEM.js";var r,i,a,o,s,c,l,u,d,f;e((()=>{t(),{fn:r}=__STORYBOOK_MODULE_TEST__,i={id:`c1`,project_id:`proj-1`,domain:`e-commerce`,definition:`A temporary container where customers collect products before purchasing.`,source:`terminology`,terms:[{text:`shopping cart`,locale:`en-US`,status:`preferred`},{text:`panier`,locale:`fr-FR`,status:`preferred`},{text:`Warenkorb`,locale:`de-DE`,status:`preferred`},{text:`ショッピングカート`,locale:`ja-JP`,status:`preferred`}],properties:{},created_at:`2026-03-15T10:00:00Z`,updated_at:`2026-03-20T14:30:00Z`},a={id:`c2`,project_id:``,domain:`legal`,definition:`A legally binding agreement between two or more parties.`,source:`terminology`,terms:[{text:`contract`,locale:`en-US`,status:`preferred`},{text:`agreement`,locale:`en-US`,status:`admitted`,note:`informal`},{text:`contrat`,locale:`fr-FR`,status:`preferred`},{text:`accord`,locale:`fr-FR`,status:`deprecated`,note:`use contrat instead`},{text:`Vertrag`,locale:`de-DE`,status:`approved`}],properties:{},created_at:`2026-02-10T10:00:00Z`,updated_at:`2026-02-10T10:00:00Z`},o={title:`Resource Browser/ConceptCard`,component:n,tags:[`autodocs`],args:{onToggleSelect:r(),onEdit:r(),onDelete:r(),onDeleteConfirm:r(),onDeleteCancel:r()},parameters:{docs:{description:{component:`Termbase concept card with reference language term, target translations, domain badge, and hover actions. Built on shadcn Card/Badge/Button.`}}}},s={args:{concept:i,referenceLocale:`en-US`}},c={args:{concept:i,referenceLocale:`en-US`,selected:!0}},l={name:`Multiple Statuses`,args:{concept:a,referenceLocale:`en-US`}},u={name:`No Reference Locale (first term)`,args:{concept:i}},d={name:`Delete Confirmation`,args:{concept:i,referenceLocale:`en-US`,deleteConfirm:!0}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    concept: sampleConcept,
    referenceLocale: "en-US"
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    concept: sampleConcept,
    referenceLocale: "en-US",
    selected: true
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  name: "Multiple Statuses",
  args: {
    concept: multiStatusConcept,
    referenceLocale: "en-US"
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  name: "No Reference Locale (first term)",
  args: {
    concept: sampleConcept
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  name: "Delete Confirmation",
  args: {
    concept: sampleConcept,
    referenceLocale: "en-US",
    deleteConfirm: true
  }
}`,...d.parameters?.docs?.source}}},f=[`Default`,`Selected`,`MultipleStatuses`,`NoReferenceLocale`,`DeleteConfirm`]}))();export{s as Default,d as DeleteConfirm,l as MultipleStatuses,u as NoReferenceLocale,c as Selected,f as __namedExportsOrder,o as default};