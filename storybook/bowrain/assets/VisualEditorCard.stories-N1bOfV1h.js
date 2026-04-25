import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,t as a}from"./VisualEditorCard-CkWPN-uY.js";import{A as o,F as s,I as c,M as l,R as u,j as d,l as f,z as p}from"./fixtures-D8OXoGLB.js";function m(e){let[t,n]=(0,h.useState)(!1),[r,i]=(0,h.useState)(e.editorMode??`translate`);return(0,g.jsx)(a,{block:e.block??v,blockIndex:e.blockIndex??1,totalBlocks:e.totalBlocks??l.length,targetLocale:e.targetLocale??`fr-FR`,editorMode:r,onEditorModeChange:i,isEditing:t,onStartEditing:()=>n(!0),onSave:(e,t)=>n(!1),onCancel:()=>n(!1),onApprove:()=>n(!1),onReject:()=>{},tmMatches:e.tmMatches??[],termMatches:e.termMatches??[],onApplyTM:e.onApplyTM??(()=>{}),onInsertTerm:e.onInsertTerm??(()=>{}),project:e.project??s,referenceLocales:e.referenceLocales,qaIssues:e.qaIssues,history:e.history,onRevertHistory:e.onRevertHistory,notes:e.notes,onAddNote:e.onAddNote,onDeleteNote:e.onDeleteNote,onTermCreate:e.onTermCreate})}var h,g,_,v,y,b,x,S,C,w,T,E,D,O,k,A,j;t((()=>{h=e(n(),1),i(),f(),g=r(),{fn:_}=__STORYBOOK_MODULE_TEST__,v=l[1],y={title:`Editor/Visual/VisualEditorCard`,component:a,tags:[`autodocs`],decorators:[e=>(0,g.jsx)(`div`,{style:{maxWidth:700,padding:24},children:(0,g.jsx)(e,{})})],args:{block:v,blockIndex:1,totalBlocks:l.length,targetLocale:`fr-FR`,editorMode:`translate`,onEditorModeChange:_(),isEditing:!1,onStartEditing:_(),onSave:_(),onCancel:_(),onApprove:_(),onReject:_(),tmMatches:[],termMatches:[],onApplyTM:_(),onInsertTerm:_(),project:s}},b={render:()=>(0,g.jsx)(m,{})},x={render:()=>(0,g.jsx)(m,{tmMatches:u})},S={render:()=>(0,g.jsx)(m,{editorMode:`enrich`,notes:d,onAddNote:_(),onDeleteNote:_(),onTermCreate:_()})},C={render:()=>(0,g.jsx)(m,{editorMode:`review`,block:l[0],blockIndex:0})},w={render:()=>(0,g.jsx)(m,{block:l[0],blockIndex:0,tmMatches:u,termMatches:p,qaIssues:c,history:o,onRevertHistory:_(),referenceLocales:[`de-DE`],notes:d,onAddNote:_(),onDeleteNote:_(),onTermCreate:_()})},T={args:{isEditing:!0}},E={args:{qaIssues:c}},D={args:{history:o,onRevertHistory:_()}},O={args:{referenceLocales:[`de-DE`],block:l[0],blockIndex:0}},k={args:{block:l[2],blockIndex:2}},A={args:{block:{...l[0],properties:{"translation-status":`reviewed`}},blockIndex:0}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveCard />
}`,...b.parameters?.docs?.source},description:{story:`Interactive card — click the target area to start editing, type text,
press Enter or click Save to confirm, Escape to cancel.
Use the mode switcher to toggle between translate / review / enrich.`,...b.parameters?.docs?.description}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveCard tmMatches={sampleTMMatches} />
}`,...x.parameters?.docs?.source},description:{story:`Interactive card with TM matches panel visible`,...x.parameters?.docs?.description}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveCard editorMode="enrich" notes={sampleBlockNotes} onAddNote={fn()} onDeleteNote={fn()} onTermCreate={fn()} />
}`,...S.parameters?.docs?.source},description:{story:`Interactive card in enrich mode with notes`,...S.parameters?.docs?.description}}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveCard editorMode="review" block={sampleBlocks[0]} blockIndex={0} />
}`,...C.parameters?.docs?.source},description:{story:`Interactive card in review mode — approve/reject are functional`,...C.parameters?.docs?.description}}},w.parameters={...w.parameters,docs:{...w.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveCard block={sampleBlocks[0]} blockIndex={0} tmMatches={sampleTMMatches} termMatches={sampleTermMatches} qaIssues={sampleQAIssues} history={sampleBlockHistory} onRevertHistory={fn()} referenceLocales={["de-DE"]} notes={sampleBlockNotes} onAddNote={fn()} onDeleteNote={fn()} onTermCreate={fn()} />
}`,...w.parameters?.docs?.source},description:{story:`Interactive card with all panels: TM, QA, history, terms, ref locales.
Full editing flow is functional.`,...w.parameters?.docs?.description}}},T.parameters={...T.parameters,docs:{...T.parameters?.docs,source:{originalSource:`{
  args: {
    isEditing: true
  }
}`,...T.parameters?.docs?.source},description:{story:`Static snapshot: translate mode with editing active`,...T.parameters?.docs?.description}}},E.parameters={...E.parameters,docs:{...E.parameters?.docs,source:{originalSource:`{
  args: {
    qaIssues: sampleQAIssues
  }
}`,...E.parameters?.docs?.source},description:{story:`Static snapshot: QA issues badge display`,...E.parameters?.docs?.description}}},D.parameters={...D.parameters,docs:{...D.parameters?.docs,source:{originalSource:`{
  args: {
    history: sampleBlockHistory,
    onRevertHistory: fn()
  }
}`,...D.parameters?.docs?.source},description:{story:`Static snapshot: block history entries`,...D.parameters?.docs?.description}}},O.parameters={...O.parameters,docs:{...O.parameters?.docs,source:{originalSource:`{
  args: {
    referenceLocales: ["de-DE"],
    block: sampleBlocks[0],
    // has de-DE target
    blockIndex: 0
  }
}`,...O.parameters?.docs?.source},description:{story:`Static snapshot: reference locales display`,...O.parameters?.docs?.description}}},k.parameters={...k.parameters,docs:{...k.parameters?.docs,source:{originalSource:`{
  args: {
    block: sampleBlocks[2],
    // empty targets
    blockIndex: 2
  }
}`,...k.parameters?.docs?.source},description:{story:`Static snapshot: not-started block (no target text)`,...k.parameters?.docs?.description}}},A.parameters={...A.parameters,docs:{...A.parameters?.docs,source:{originalSource:`{
  args: {
    block: {
      ...sampleBlocks[0],
      properties: {
        "translation-status": "reviewed"
      }
    },
    blockIndex: 0
  }
}`,...A.parameters?.docs?.source},description:{story:`Static snapshot: reviewed block status`,...A.parameters?.docs?.description}}},j=[`Interactive`,`WithTMMatches`,`EnrichMode`,`ReviewMode`,`FullFeatured`,`TranslateModeEditing`,`WithQAIssues`,`WithHistory`,`WithReferenceLocales`,`NotStartedBlock`,`ReviewedBlock`]}))();export{S as EnrichMode,w as FullFeatured,b as Interactive,k as NotStartedBlock,C as ReviewMode,A as ReviewedBlock,T as TranslateModeEditing,D as WithHistory,E as WithQAIssues,O as WithReferenceLocales,x as WithTMMatches,j as __namedExportsOrder,y as default};