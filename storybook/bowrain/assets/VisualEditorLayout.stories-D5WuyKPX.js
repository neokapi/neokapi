import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,t as a}from"./VisualEditorLayout-DQsPZLFi.js";import{A as o,F as s,I as c,M as l,P as u,R as d,j as f,l as p,w as m,z as h}from"./fixtures-D8OXoGLB.js";import{n as g,r as _,t as v}from"./decorators-BptasVUO.js";function y(e){let t=e.blocks??l,[n,r]=(0,x.useState)(e.selectedIndex??0),[i,o]=(0,x.useState)(null),[c,u]=(0,x.useState)(e.editorMode??`translate`),[d,f]=(0,x.useState)(e.previewContentMode??`source`),p=(0,x.useCallback)(e=>{r(e),o(null)},[]),m=(0,x.useCallback)(()=>{o(n)},[n]),h=(0,x.useCallback)((e,n)=>{o(null),r(e=>Math.min(e+1,t.length-1))},[t.length]),g=(0,x.useCallback)(()=>{o(null)},[]);return(0,S.jsx)(a,{project:e.project??s,fileName:e.fileName??`messages.json`,blocks:t,selectedIndex:n,editingIndex:i,targetLocale:e.targetLocale??`fr-FR`,editorMode:c,onEditorModeChange:u,previewContentMode:d,onPreviewContentModeChange:f,onNavigate:p,onStartEditing:m,onSave:h,onCancelEditing:g,onApprove:()=>r(e=>Math.min(e+1,t.length-1)),onReject:()=>{},tmMatches:e.tmMatches??[],termMatches:e.termMatches??[],onApplyTM:e.onApplyTM??(()=>{}),onInsertTerm:e.onInsertTerm??(()=>{}),presenceSlot:e.presenceSlot,qaIssues:e.qaIssues,fileQAResults:e.fileQAResults,qaLoading:e.qaLoading,onRunFileQA:e.onRunFileQA,history:e.history,onRevertHistory:e.onRevertHistory,notes:e.notes,onAddNote:e.onAddNote,onDeleteNote:e.onDeleteNote,onTermCreate:e.onTermCreate})}function b(){let e=m,[t,n]=(0,x.useState)(0),[r,i]=(0,x.useState)(null),[o,c]=(0,x.useState)(`translate`),[l,u]=(0,x.useState)(`source`);return(0,S.jsx)(a,{project:s,fileName:`getting-started.md`,blocks:e,selectedIndex:t,editingIndex:r,targetLocale:`fr-FR`,editorMode:o,onEditorModeChange:c,previewContentMode:l,onPreviewContentModeChange:u,onNavigate:(0,x.useCallback)(e=>{n(e),i(null)},[]),onStartEditing:(0,x.useCallback)(()=>{i(t)},[t]),onSave:(0,x.useCallback)((t,r)=>{i(null),n(t=>Math.min(t+1,e.length-1))},[e.length]),onCancelEditing:(0,x.useCallback)(()=>{i(null)},[]),onApprove:()=>n(t=>Math.min(t+1,e.length-1)),onReject:()=>{},tmMatches:[],termMatches:[],onApplyTM:()=>{},onInsertTerm:()=>{}})}var x,S,C,w,T,E,D,O,k,A,j,M,N,P,F,I,L,R;t((()=>{x=e(n(),1),i(),p(),g(),S=r(),{fn:C}=__STORYBOOK_MODULE_TEST__,w={title:`Editor/Visual/VisualEditorLayout`,component:a,tags:[`autodocs`],decorators:[_,e=>(0,S.jsx)(`div`,{style:{width:`100vw`,height:`100vh`,overflow:`auto`},children:(0,S.jsx)(e,{})})],parameters:{layout:`fullscreen`},args:{project:s,fileName:`messages.json`,blocks:l,selectedIndex:0,editingIndex:null,targetLocale:`fr-FR`,editorMode:`translate`,onEditorModeChange:C(),previewContentMode:`source`,onPreviewContentModeChange:C(),onNavigate:C(),onStartEditing:C(),onSave:C(),onCancelEditing:C(),onApprove:C(),onReject:C(),tmMatches:[],termMatches:[],onApplyTM:C(),onInsertTerm:C()}},T={render:()=>(0,S.jsx)(y,{})},E={render:()=>(0,S.jsx)(y,{tmMatches:d})},D={render:()=>(0,S.jsx)(y,{editorMode:`review`})},O={render:()=>(0,S.jsx)(y,{editorMode:`enrich`,notes:f,onAddNote:C(),onDeleteNote:C(),onTermCreate:C()})},k={render:()=>(0,S.jsx)(y,{selectedIndex:1,tmMatches:d,termMatches:h,qaIssues:c,fileQAResults:u,onRunFileQA:C(),history:o,onRevertHistory:C(),notes:f,onAddNote:C(),onDeleteNote:C(),onTermCreate:C(),presenceSlot:(0,S.jsxs)(`div`,{style:{display:`flex`,gap:4},children:[(0,S.jsx)(`div`,{style:{width:24,height:24,borderRadius:`50%`,background:`#6366f1`,display:`flex`,alignItems:`center`,justifyContent:`center`,color:`#fff`,fontSize:11,fontWeight:600},children:`JD`}),(0,S.jsx)(`div`,{style:{width:24,height:24,borderRadius:`50%`,background:`#f59e0b`,display:`flex`,alignItems:`center`,justifyContent:`center`,color:`#fff`,fontSize:11,fontWeight:600},children:`MK`})]})})},A={args:{selectedIndex:1}},j={args:{termMatches:h}},M={args:{qaIssues:c,fileQAResults:u,onRunFileQA:C()}},N={args:{history:o,onRevertHistory:C()}},P={args:{previewContentMode:`target`}},F={args:{previewContentMode:`pseudo`}},I={render:()=>(0,S.jsx)(y,{previewContentMode:`pseudo`})},L={decorators:[v(m),e=>(0,S.jsx)(`div`,{style:{width:`100vw`,height:`100vh`,overflow:`auto`},children:(0,S.jsx)(e,{})})],render:()=>(0,S.jsx)(b,{})},T.parameters={...T.parameters,docs:{...T.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveLayout />
}`,...T.parameters?.docs?.source},description:{story:`Interactive layout — click blocks in the card list or preview to navigate,
Enter to start editing, Escape to cancel, switch modes via the toolbar.`,...T.parameters?.docs?.description}}},E.parameters={...E.parameters,docs:{...E.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveLayout tmMatches={sampleTMMatches} />
}`,...E.parameters?.docs?.source},description:{story:`Interactive layout with TM matches`,...E.parameters?.docs?.description}}},D.parameters={...D.parameters,docs:{...D.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveLayout editorMode="review" />
}`,...D.parameters?.docs?.source},description:{story:`Interactive layout in review mode`,...D.parameters?.docs?.description}}},O.parameters={...O.parameters,docs:{...O.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveLayout editorMode="enrich" notes={sampleBlockNotes} onAddNote={fn()} onDeleteNote={fn()} onTermCreate={fn()} />
}`,...O.parameters?.docs?.source},description:{story:`Interactive layout in enrich mode with notes and term creation`,...O.parameters?.docs?.description}}},k.parameters={...k.parameters,docs:{...k.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveLayout selectedIndex={1} tmMatches={sampleTMMatches} termMatches={sampleTermMatches} qaIssues={sampleQAIssues} fileQAResults={sampleFileQAResults} onRunFileQA={fn()} history={sampleBlockHistory} onRevertHistory={fn()} notes={sampleBlockNotes} onAddNote={fn()} onDeleteNote={fn()} onTermCreate={fn()} presenceSlot={<div style={{
    display: "flex",
    gap: 4
  }}>
          <div style={{
      width: 24,
      height: 24,
      borderRadius: "50%",
      background: "#6366f1",
      display: "flex",
      alignItems: "center",
      justifyContent: "center",
      color: "#fff",
      fontSize: 11,
      fontWeight: 600
    }}>
            JD
          </div>
          <div style={{
      width: 24,
      height: 24,
      borderRadius: "50%",
      background: "#f59e0b",
      display: "flex",
      alignItems: "center",
      justifyContent: "center",
      color: "#fff",
      fontSize: 11,
      fontWeight: 600
    }}>
            MK
          </div>
        </div>} />
}`,...k.parameters?.docs?.source},description:{story:`Interactive layout with all panels: TM, terms, QA, history, notes,
presence slot. Full editing flow is functional.`,...k.parameters?.docs?.description}}},A.parameters={...A.parameters,docs:{...A.parameters?.docs,source:{originalSource:`{
  args: {
    selectedIndex: 1
  }
}`,...A.parameters?.docs?.source},description:{story:`Static snapshot: second block selected`,...A.parameters?.docs?.description}}},j.parameters={...j.parameters,docs:{...j.parameters?.docs,source:{originalSource:`{
  args: {
    termMatches: sampleTermMatches
  }
}`,...j.parameters?.docs?.source},description:{story:`Static snapshot: term sidebar visible`,...j.parameters?.docs?.description}}},M.parameters={...M.parameters,docs:{...M.parameters?.docs,source:{originalSource:`{
  args: {
    qaIssues: sampleQAIssues,
    fileQAResults: sampleFileQAResults,
    onRunFileQA: fn()
  }
}`,...M.parameters?.docs?.source},description:{story:`Static snapshot: QA issues and file QA results`,...M.parameters?.docs?.description}}},N.parameters={...N.parameters,docs:{...N.parameters?.docs,source:{originalSource:`{
  args: {
    history: sampleBlockHistory,
    onRevertHistory: fn()
  }
}`,...N.parameters?.docs?.source},description:{story:`Static snapshot: block history`,...N.parameters?.docs?.description}}},P.parameters={...P.parameters,docs:{...P.parameters?.docs,source:{originalSource:`{
  args: {
    previewContentMode: "target"
  }
}`,...P.parameters?.docs?.source},description:{story:`Static snapshot: target preview content mode`,...P.parameters?.docs?.description}}},F.parameters={...F.parameters,docs:{...F.parameters?.docs,source:{originalSource:`{
  args: {
    previewContentMode: "pseudo"
  }
}`,...F.parameters?.docs?.source},description:{story:`Static snapshot: pseudo preview — source text with accent mapping`,...F.parameters?.docs?.description}}},I.parameters={...I.parameters,docs:{...I.parameters?.docs,source:{originalSource:`{
  render: () => <InteractiveLayout previewContentMode="pseudo" />
}`,...I.parameters?.docs?.source},description:{story:`Interactive pseudo mode — click blocks, switch modes, see accented source`,...I.parameters?.docs?.description}}},L.parameters={...L.parameters,docs:{...L.parameters?.docs,source:{originalSource:`{
  decorators: [createProvidersDecorator(navigationBlocks), Story => <div style={{
    width: "100vw",
    height: "100vh",
    overflow: "auto"
  }}>
        <Story />
      </div>],
  render: () => <NavigationDemo />
}`,...L.parameters?.docs?.source},description:{story:`Interactive navigation story — use keyboard shortcuts to move between blocks:
- **j / ArrowDown** — next block
- **k / ArrowUp** — previous block
- **Enter** — start editing
- **Escape** — cancel editing
- **n / N** — next / previous untranslated block

Click blocks in the preview to jump directly.`,...L.parameters?.docs?.description}}},R=[`Default`,`WithTMMatches`,`ReviewMode`,`EnrichMode`,`FullFeatured`,`SecondBlockSelected`,`WithTermSidebar`,`WithQAIssues`,`WithHistory`,`TargetPreview`,`PseudoPreview`,`PseudoMode`,`Navigation`]}))();export{T as Default,O as EnrichMode,k as FullFeatured,L as Navigation,I as PseudoMode,F as PseudoPreview,D as ReviewMode,A as SecondBlockSelected,P as TargetPreview,N as WithHistory,M as WithQAIssues,E as WithTMMatches,j as WithTermSidebar,R as __namedExportsOrder,w as default};