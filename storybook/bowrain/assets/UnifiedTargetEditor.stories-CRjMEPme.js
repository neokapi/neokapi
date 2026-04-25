import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,t as a}from"./UnifiedTargetEditor-BNY01EM0.js";function o(e){return{id:`blk-messages`,source:`You have {count} messages`,has_spans:!0,source_spans:[{span_type:`placeholder`,type:`jsx:var`,id:`0`,data:`{count}`,equiv_text:`count`}],targets:e,targets_coded:{},translatable:!0,properties:{}}}function s(e){return{id:`blk-rich`,source:`Click <strong>here</strong> for {count} pending.`,has_spans:!0,source_spans:[{span_type:`opening`,type:`fmt:bold`,id:`0`,data:`<strong>`,equiv_text:`strong`},{span_type:`closing`,type:`fmt:bold`,id:`0`,data:`</strong>`,equiv_text:`strong`},{span_type:`placeholder`,type:`jsx:var`,id:`1`,data:`{count}`,equiv_text:`count`}],targets:e,targets_coded:{},translatable:!0,properties:{}}}function c(e){return{id:`blk-plain`,source:`Welcome back!`,has_spans:!1,source_spans:[],targets:e,targets_coded:{},translatable:!0,properties:{}}}function l({block:e,locale:t=`de`}){let[n,r]=(0,u.useState)(null),[i,o]=(0,u.useState)(null),[s,c]=(0,u.useState)(!0);return(0,d.jsxs)(`div`,{style:{minHeight:500,padding:16,fontFamily:`sans-serif`},children:[(0,d.jsxs)(`div`,{style:{marginBottom:12,fontSize:14},children:[(0,d.jsx)(`strong`,{children:`Source:`}),` `,(0,d.jsx)(`code`,{children:e.source})]}),(0,d.jsxs)(`div`,{style:{marginBottom:12,fontSize:14},children:[(0,d.jsxs)(`strong`,{children:[`Target (`,t,`):`]}),` `,(0,d.jsx)(`code`,{children:e.targets[t]??`(empty)`})]}),s?(0,d.jsx)(a,{block:e,locale:t,onSave:e=>{e.kind===`flat`?r({codedText:e.codedText,spansCount:e.spans.length}):o(e.text),c(!1)},onCancel:()=>c(!1)}):(0,d.jsx)(`button`,{type:`button`,onClick:()=>c(!0),children:`Reopen editor`}),n&&(0,d.jsxs)(`pre`,{style:{background:`#f4f4f5`,padding:8,borderRadius:4,marginTop:12},children:[`flat saved → codedText=`,JSON.stringify(n.codedText),` spans=`,n.spansCount]}),i&&(0,d.jsxs)(`pre`,{style:{background:`#f4f4f5`,padding:8,borderRadius:4,marginTop:12,whiteSpace:`pre-wrap`},children:[`plural saved → `,i]})]})}var u,d,f,p,m,h,g,_,v,y,b;t((()=>{u=e(n(),1),i(),d=r(),f={title:`Editor/Core/UnifiedTargetEditor`,component:a,tags:[`autodocs`],parameters:{docs:{description:{component:`Single editor surface for every target — flat or plural, with inline codes or plain. Replaces TargetCellEditor + the textarea fallback + the Plurals dialog. Lexical chips render identically across modes; plural authoring is a mode toggle inside the editor. See AD #408 / #409.`}}}},p={name:`Flat target with placeholder`,render:()=>(0,d.jsx)(l,{block:o({de:`Sie haben {count} Nachrichten`})})},m={name:`Flat target with paired inline codes`,render:()=>(0,d.jsx)(l,{block:s({de:`Klicken Sie hier — {count} ausstehend.`})})},h={name:`Flat plain text (no spans)`,render:()=>(0,d.jsx)(l,{block:c({de:`Willkommen zurück!`})})},g={name:`Plural target — opens in per-form view`,render:()=>(0,d.jsx)(l,{block:o({de:`{count, plural, one {Sie haben 1 Nachricht} other {Sie haben {count} Nachrichten}}`})})},_={name:`Plural target with paired inline codes`,render:()=>(0,d.jsx)(l,{block:s({de:`{count, plural, one {Klicken Sie {=strong}hier{/=strong} — 1 ausstehend.} other {Klicken Sie {=strong}hier{/=strong} — {count} ausstehend.}}`})})},v={name:`Empty target — author flat, then upgrade`,render:()=>(0,d.jsx)(l,{block:o({})})},y={name:`No pivot candidates — upgrade button hidden`,render:()=>(0,d.jsx)(l,{block:c({})})},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  name: "Flat target with placeholder",
  render: () => <Wrapper block={makeMessagesBlock({
    de: "Sie haben {count} Nachrichten"
  })} />
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "Flat target with paired inline codes",
  render: () => <Wrapper block={makeRichBlock({
    de: "Klicken Sie hier — {count} ausstehend."
  })} />
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "Flat plain text (no spans)",
  render: () => <Wrapper block={makePlainBlock({
    de: "Willkommen zurück!"
  })} />
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "Plural target — opens in per-form view",
  render: () => <Wrapper block={makeMessagesBlock({
    de: "{count, plural, one {Sie haben 1 Nachricht} other {Sie haben {count} Nachrichten}}"
  })} />
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "Plural target with paired inline codes",
  render: () => <Wrapper block={makeRichBlock({
    de: "{count, plural, one {Klicken Sie {=strong}hier{/=strong} — 1 ausstehend.} other {Klicken Sie {=strong}hier{/=strong} — {count} ausstehend.}}"
  })} />
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  name: "Empty target — author flat, then upgrade",
  render: () => <Wrapper block={makeMessagesBlock({})} />
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  name: "No pivot candidates — upgrade button hidden",
  render: () => <Wrapper block={makePlainBlock({})} />
}`,...y.parameters?.docs?.source}}},b=[`FlatPlaceholder`,`FlatRichInlineCodes`,`FlatPlainText`,`PluralTarget`,`PluralWithInlineCodes`,`EmptyFlatThenUpgrade`,`NoPivotCandidates`]}))();export{v as EmptyFlatThenUpgrade,p as FlatPlaceholder,h as FlatPlainText,m as FlatRichInlineCodes,y as NoPivotCandidates,g as PluralTarget,_ as PluralWithInlineCodes,b as __namedExportsOrder,f as default};