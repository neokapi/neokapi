import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,r as a,t as o}from"./runtime-B587fM_0.js";import{St as s,j as c,m as l,t as u,xt as d,z as f}from"./lucide-react-CNTu2eXS.js";import{t as p}from"./src-B5GKo4pn.js";import{t as m}from"./button-Dd5Xmast.js";import{t as h}from"./card-DEFJwrtm.js";import{i as g}from"./TMFacetSidebar-BYYSd6v4.js";import{n as _,t as v}from"./PipelineProgress-BKvuaaWC.js";function y({flowName:e,flow:t,autoRun:n=!1,simulateError:r=!1,fileCount:a=3,stepDurationMs:u=800}){let[p,_]=(0,b.useState)(`idle`),[y,S]=(0,b.useState)({current:0,total:a}),[C,w]=(0,b.useState)(``),[T,E]=(0,b.useState)(t.steps.map(e=>({name:e.tool,parts_in:0,parts_out:0}))),[D,O]=(0,b.useState)([]),[k,A]=(0,b.useState)(0),j=(0,b.useCallback)((e,t)=>{O(n=>[...n,{type:e,message:t,ts:Date.now()}])},[]),M=(0,b.useCallback)(async()=>{_(`running`),S({current:0,total:a}),E(t.steps.map(e=>({name:e.tool,parts_in:0,parts_out:0}))),O([]),A(0);let e=Date.now(),n=Array.from({length:a},(e,t)=>`src/locales/en/messages-${t+1}.json`);j(`state`,`Flow execution started`);for(let i=0;i<n.length;i++){let a=n[i];w(a),S({current:i,total:n.length}),j(`progress`,`Processing ${a}`);for(let n=0;n<t.steps.length;n++)if(j(`trace`,`  ${t.steps[n].tool}: processing ${a}`),E(e=>e.map((e,t)=>t===n?{...e,parts_in:e.parts_in+30}:e)),await new Promise(e=>setTimeout(e,u)),A(Date.now()-e),E(e=>e.map((e,t)=>t===n?{...e,parts_out:e.parts_out+30}:e)),r&&i===1&&n===1){j(`error`,`Error in ${t.steps[n].tool}: connection timeout to AI provider`),_(`error`);return}S({current:i+1,total:n.length}),j(`complete`,`Completed ${a}`)}w(``);let i=((Date.now()-e)/1e3).toFixed(1);j(`complete`,`Flow completed: ${n.length} files in ${i}s`),_(`complete`)},[t,a,u,r,j]),N=(0,b.useCallback)(()=>{_(`canceled`),j(`state`,`Flow execution canceled by user`)},[j]);(0,b.useEffect)(()=>{if(n){let e=setTimeout(M,500);return()=>clearTimeout(e)}},[n,M]);let P={idle:null,running:(0,x.jsx)(f,{size:16,className:`animate-spin text-primary`}),complete:(0,x.jsx)(s,{size:16,className:`text-green-500`}),error:(0,x.jsx)(d,{size:16,className:`text-destructive`}),canceled:(0,x.jsx)(d,{size:16,className:`text-muted-foreground`})},F=y.total>0?Math.round(y.current/y.total*100):0;return(0,x.jsxs)(`div`,{className:`p-6`,children:[(0,x.jsxs)(`div`,{className:`mb-4 flex items-center gap-3`,children:[(0,x.jsx)(`h2`,{className:`text-lg font-medium`,children:o(`Sog9D`,`Run: ${e}`,{flowName:e})}),P[p],p===`running`&&k>0&&(0,x.jsx)(`span`,{className:`text-xs text-muted-foreground`,children:o(`Q1heW`,`${(k/1e3).toFixed(1)}s`,{toFixed:(k/1e3).toFixed(1)})})]}),(0,x.jsxs)(h,{className:`mb-4 p-3`,children:[(0,x.jsx)(`h3`,{className:`mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground`,children:o(`1e2bv`,`Pipeline`)}),(0,x.jsx)(v,{steps:t.steps,snapshots:T,runState:p})]}),(0,x.jsxs)(`div`,{className:`mb-4 flex gap-2`,children:[p===`idle`&&(0,x.jsx)(m,{onClick:M,"data-testid":`run-button`,children:i(`cz6CH`,`{=m0} Run Flow`,{"=m0":(0,x.jsx)(c,{size:14})})}),p===`running`&&(0,x.jsx)(m,{variant:`outline`,onClick:N,className:`border-destructive text-destructive hover:bg-destructive/10`,children:i(`1gNpf4`,`{=m0} Cancel`,{"=m0":(0,x.jsx)(l,{size:14})})}),(p===`complete`||p===`error`||p===`canceled`)&&(0,x.jsx)(m,{variant:`outline`,onClick:()=>{_(`idle`),O([]),S({current:0,total:a}),setActiveStep(-1)},children:o(`37rMfi`,`Reset`)})]}),(p===`running`||p===`complete`)&&(0,x.jsxs)(`div`,{className:`mb-4`,children:[(0,x.jsx)(`div`,{className:`mb-1 flex justify-between text-xs text-muted-foreground`,children:i(`pKrtN`,`{=m0} {=m1}`,{"=m0":(0,x.jsx)(`span`,{children:C||`${y.current} / ${y.total} files`}),"=m1":(0,x.jsxs)(`span`,{children:[F,`%`]})})}),(0,x.jsx)(`div`,{className:`h-2 overflow-hidden rounded-full bg-accent`,role:`progressbar`,"aria-valuenow":y.current,"aria-valuemin":0,"aria-valuemax":y.total,children:(0,x.jsx)(`div`,{className:`h-full rounded-full bg-primary transition-all duration-500`,style:{width:`${F}%`}})})]}),D.length>0&&(0,x.jsxs)(h,{children:[(0,x.jsx)(`h3`,{className:`border-b border-border px-3 py-2 text-xs font-medium uppercase tracking-wide text-muted-foreground`,children:o(`21fpAx`,`Output (${D.length} events)`,{"events.length":D.length})}),(0,x.jsx)(g,{className:`max-h-48`,children:(0,x.jsx)(`div`,{className:`p-3 font-mono text-xs`,children:D.map((e,t)=>(0,x.jsx)(`div`,{className:`py-0.5 ${e.type===`error`?`text-destructive`:e.type===`complete`?`text-green-500`:e.type===`trace`?`text-muted-foreground`:`text-foreground`}`,children:e.message},t))})})]})]})}var b,x,S,C,w,T,E,D,O;t((()=>{a(),b=e(n(),1),u(),p(),_(),x=r(),S={title:`Interactions/Flow Execution`,component:y,tags:[`autodocs`],parameters:{docs:{description:{component:`Demonstrates flow execution with live progress bars, step highlighting, event streaming, error handling, and cancellation.`}}}},C={args:{flowName:`translate-and-qa`,flow:{steps:[{tool:`ai-translate`,config:{provider:`anthropic`}},{tool:`qa-check`}]}}},w={name:`Auto-run (3 files, 2 steps)`,args:{flowName:`translate-and-qa`,flow:{steps:[{tool:`ai-translate`},{tool:`qa-check`}]},autoRun:!0,fileCount:3,stepDurationMs:600}},T={name:`Long pipeline (5 files, 4 steps)`,args:{flowName:`full-pipeline`,flow:{steps:[{tool:`tm-leverage`},{tool:`ai-translate`},{tool:`qa-check`},{tool:`ai-qa`}]},autoRun:!0,fileCount:5,stepDurationMs:400}},E={name:`Error during execution`,args:{flowName:`translate-and-qa`,flow:{steps:[{tool:`ai-translate`},{tool:`qa-check`}]},autoRun:!0,simulateError:!0,fileCount:3,stepDurationMs:500}},D={name:`Single file (fast)`,args:{flowName:`pseudo`,flow:{steps:[{tool:`pseudo-translate`}]},autoRun:!0,fileCount:1,stepDurationMs:300}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  args: {
    flowName: "translate-and-qa",
    flow: {
      steps: [{
        tool: "ai-translate",
        config: {
          provider: "anthropic"
        }
      }, {
        tool: "qa-check"
      }]
    }
  }
}`,...C.parameters?.docs?.source}}},w.parameters={...w.parameters,docs:{...w.parameters?.docs,source:{originalSource:`{
  name: "Auto-run (3 files, 2 steps)",
  args: {
    flowName: "translate-and-qa",
    flow: {
      steps: [{
        tool: "ai-translate"
      }, {
        tool: "qa-check"
      }]
    },
    autoRun: true,
    fileCount: 3,
    stepDurationMs: 600
  }
}`,...w.parameters?.docs?.source}}},T.parameters={...T.parameters,docs:{...T.parameters?.docs,source:{originalSource:`{
  name: "Long pipeline (5 files, 4 steps)",
  args: {
    flowName: "full-pipeline",
    flow: {
      steps: [{
        tool: "tm-leverage"
      }, {
        tool: "ai-translate"
      }, {
        tool: "qa-check"
      }, {
        tool: "ai-qa"
      }]
    },
    autoRun: true,
    fileCount: 5,
    stepDurationMs: 400
  }
}`,...T.parameters?.docs?.source}}},E.parameters={...E.parameters,docs:{...E.parameters?.docs,source:{originalSource:`{
  name: "Error during execution",
  args: {
    flowName: "translate-and-qa",
    flow: {
      steps: [{
        tool: "ai-translate"
      }, {
        tool: "qa-check"
      }]
    },
    autoRun: true,
    simulateError: true,
    fileCount: 3,
    stepDurationMs: 500
  }
}`,...E.parameters?.docs?.source}}},D.parameters={...D.parameters,docs:{...D.parameters?.docs,source:{originalSource:`{
  name: "Single file (fast)",
  args: {
    flowName: "pseudo",
    flow: {
      steps: [{
        tool: "pseudo-translate"
      }]
    },
    autoRun: true,
    fileCount: 1,
    stepDurationMs: 300
  }
}`,...D.parameters?.docs?.source}}},O=[`IdleState`,`AutoRunThreeFiles`,`LongPipeline`,`ErrorDuringExecution`,`SingleFileQuick`]}))();export{w as AutoRunThreeFiles,E as ErrorDuringExecution,C as IdleState,T as LongPipeline,D as SingleFileQuick,O as __namedExportsOrder,S as default};