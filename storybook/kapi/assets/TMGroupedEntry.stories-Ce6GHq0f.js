import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./TMGroupedEntry-BauShLxS.js";function i(e,t){return{locale:e,text:t,coded:t,spans:[]}}var a,o,s,c,l,u,d,f,p,m,h,g,_,v,y;e((()=>{n(),a=t(),{fn:o}=__STORYBOOK_MODULE_TEST__,s=new Date().toISOString(),c={id:`tm-basic`,project_id:`webapp`,hint_src_lang:`en-US`,variants:{"en-US":i(`en-US`,`The client library handles token refresh and session management.`),"fr-FR":i(`fr-FR`,`La bibliothèque cliente gère le renouvellement des jetons et la gestion des sessions.`),"de-DE":i(`de-DE`,`Die Client-Bibliothek übernimmt die Token-Erneuerung und Sitzungsverwaltung.`),"ja-JP":i(`ja-JP`,`クライアントライブラリはトークンの更新とセッション管理を自動的に処理します。`)},created_at:s,updated_at:s},l={id:`tm-single`,project_id:``,hint_src_lang:`en-US`,variants:{"en-US":i(`en-US`,`This action cannot be undone.`),"fr-FR":i(`fr-FR`,`Cette action ne peut pas être annulée.`)},created_at:s,updated_at:s},u={...c,id:`tm-many`,variants:{...c.variants,"it-IT":i(`it-IT`,`Il client gestisce...`),"pt-BR":i(`pt-BR`,`O cliente gerencia...`),"ar-SA":i(`ar-SA`,`مكتبة العميل...`),"ko-KR":i(`ko-KR`,`클라이언트 라이브러리...`),"zh-CN":i(`zh-CN`,`客户端库...`),"sv-SE":i(`sv-SE`,`Klientbiblioteket...`),"nb-NO":i(`nb-NO`,`Klientbiblioteket...`),"es-ES":i(`es-ES`,`La biblioteca cliente...`),"nl-NL":i(`nl-NL`,`De clientbibliotheek...`),"pl-PL":i(`pl-PL`,`Biblioteka klienta...`)}},d={title:`Resource Browser/TMGroupedEntry`,component:r,tags:[`autodocs`],decorators:[e=>(0,a.jsx)(`div`,{style:{maxWidth:700,padding:16},children:(0,a.jsx)(e,{})})],parameters:{docs:{description:{component:`Expandable card for a multilingual TM entry. The hint_src_lang variant is shown as the header; every other variant is rendered beneath.`}}}},f={args:{entry:c,selected:!1,onToggleSelect:o(),onEditVariant:o(),onDelete:o()}},p={args:{entry:c,selected:!0,onToggleSelect:o(),onEditVariant:o(),onDelete:o()}},m={args:{entry:l,selected:!1,onToggleSelect:o(),onEditVariant:o(),onDelete:o()}},h={args:{entry:c,selected:!1,visibleLocales:[`fr-FR`,`de-DE`],onToggleSelect:o(),onEditVariant:o(),onDelete:o()}},g={args:{entry:c,selected:!1,visibleLocales:[`fr-FR`],onToggleSelect:o(),onEditVariant:o(),onDelete:o()}},_={args:{entry:u,selected:!1,onToggleSelect:o(),onEditVariant:o(),onDelete:o()}},v={render:()=>(0,a.jsxs)(`div`,{className:`space-y-2`,children:[(0,a.jsx)(r,{entry:c,selected:!1,onToggleSelect:o(),onEditVariant:o(),onDelete:o()}),(0,a.jsx)(r,{entry:l,selected:!0,onToggleSelect:o(),onEditVariant:o(),onDelete:o()})]})},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    entry: basicEntry,
    selected: false,
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn()
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    entry: basicEntry,
    selected: true,
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn()
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    entry: singleTarget,
    selected: false,
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn()
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    entry: basicEntry,
    selected: false,
    visibleLocales: ["fr-FR", "de-DE"],
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn()
  }
}`,...h.parameters?.docs?.source},description:{story:`Only shows variants for the specified locales; count shows "visible/total".`,...h.parameters?.docs?.description}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    entry: basicEntry,
    selected: false,
    visibleLocales: ["fr-FR"],
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn()
  }
}`,...g.parameters?.docs?.source},description:{story:`Single locale filter — only French.`,...g.parameters?.docs?.description}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  args: {
    entry: manyTargets,
    selected: false,
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn()
  }
}`,..._.parameters?.docs?.source},description:{story:`Entries with 10+ non-source variants stay collapsed by default.`,..._.parameters?.docs?.description}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  render: () => <div className="space-y-2">
      <TMGroupedEntry entry={basicEntry} selected={false} onToggleSelect={fn()} onEditVariant={fn()} onDelete={fn()} />
      <TMGroupedEntry entry={singleTarget} selected={true} onToggleSelect={fn()} onEditVariant={fn()} onDelete={fn()} />
    </div>
}`,...v.parameters?.docs?.source}}},y=[`AutoExpanded`,`Selected`,`SingleTranslation`,`FilteredByLocale`,`SingleLocaleFilter`,`ManyTargetsCollapsed`,`MultipleEntries`]}))();export{f as AutoExpanded,h as FilteredByLocale,_ as ManyTargetsCollapsed,v as MultipleEntries,p as Selected,g as SingleLocaleFilter,m as SingleTranslation,y as __namedExportsOrder,d as default};