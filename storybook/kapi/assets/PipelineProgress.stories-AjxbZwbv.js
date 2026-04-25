import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,t as a}from"./PipelineProgress-BKvuaaWC.js";function o({steps:e,fileCount:t=3,partsPerFile:n=40,intervalMs:r=50}){let i=t*n,[o,l]=(0,c.useState)(e.map(e=>({name:e.tool,parts_in:0,parts_out:0}))),[u,d]=(0,c.useState)(`idle`),f=(0,c.useCallback)(()=>{d(`running`),l(e.map(e=>({name:e.tool,parts_in:0,parts_out:0})));let t=0,n=setInterval(()=>{t++,l(e.map((e,n)=>{let r=n*8,a=Math.min(i,Math.max(0,t*3-r)),o=Math.min(a,Math.max(0,t*3-r-5));return{name:e.tool,parts_in:a,parts_out:o}})),t*3>i+e.length*8+10&&(clearInterval(n),l(e.map(e=>({name:e.tool,parts_in:i,parts_out:i}))),d(`complete`))},r);return()=>clearInterval(n)},[e,i,r]);return(0,c.useEffect)(()=>f(),[f]),(0,s.jsxs)(`div`,{className:`space-y-4 p-4`,children:[(0,s.jsx)(a,{steps:e,snapshots:o,runState:u}),(0,s.jsx)(`p`,{className:`text-xs text-muted-foreground`,children:u===`running`?`Processing...`:u===`complete`?`Done!`:`Idle`})]})}var s,c,l,u,d,f,p,m,h,g,_,v,y,b,x,S,C;t((()=>{i(),s=r(),c=e(n(),1),l={title:`Components/Pipeline Progress`,component:a,tags:[`autodocs`],parameters:{docs:{description:{component:`Visualizes a streaming pipeline's step-by-step progress using Badge-based step indicators. Each step transitions through pending, active (with spinner and part counts), and done states based on real-time atomic counter snapshots from the backend.`}}}},u=[{tool:`ai-translate`},{tool:`qa-check`}],d=[{tool:`tm-leverage`},{tool:`ai-translate`},{tool:`qa-check`},{tool:`term-enforce`}],f={name:`Idle (all pending)`,args:{steps:u,runState:`idle`}},p={name:`Running — all pending`,args:{steps:d,runState:`running`,snapshots:[]}},m={name:`Running — first step active`,args:{steps:d,runState:`running`,snapshots:[{name:`tm-leverage`,parts_in:47,parts_out:32},{name:`ai-translate`,parts_in:0,parts_out:0},{name:`qa-check`,parts_in:0,parts_out:0},{name:`term-enforce`,parts_in:0,parts_out:0}]}},h={name:`Running — mid-pipeline`,args:{steps:d,runState:`running`,snapshots:[{name:`tm-leverage`,parts_in:120,parts_out:120},{name:`ai-translate`,parts_in:120,parts_out:87},{name:`qa-check`,parts_in:87,parts_out:52},{name:`term-enforce`,parts_in:0,parts_out:0}]}},g={name:`Running — near complete`,args:{steps:u,runState:`running`,snapshots:[{name:`ai-translate`,parts_in:120,parts_out:120},{name:`qa-check`,parts_in:120,parts_out:118}]}},_={name:`Complete`,args:{steps:d,runState:`complete`,snapshots:[{name:`tm-leverage`,parts_in:120,parts_out:120},{name:`ai-translate`,parts_in:120,parts_out:120},{name:`qa-check`,parts_in:120,parts_out:120},{name:`term-enforce`,parts_in:120,parts_out:120}]}},v={name:`Error (frozen mid-run)`,args:{steps:d,runState:`error`,snapshots:[{name:`tm-leverage`,parts_in:120,parts_out:120},{name:`ai-translate`,parts_in:45,parts_out:12},{name:`qa-check`,parts_in:0,parts_out:0},{name:`term-enforce`,parts_in:0,parts_out:0}]}},y={name:`Canceled`,args:{steps:u,runState:`canceled`,snapshots:[{name:`ai-translate`,parts_in:60,parts_out:33},{name:`qa-check`,parts_in:0,parts_out:0}]}},b={name:`Single step`,args:{steps:[{tool:`pseudo-translate`}],runState:`running`,snapshots:[{name:`pseudo-translate`,parts_in:42,parts_out:17}]}},x={name:`Long pipeline (6 steps)`,args:{steps:[{tool:`tm-leverage`},{tool:`term-lookup`},{tool:`ai-translate`},{tool:`qa-check`},{tool:`term-enforce`},{tool:`tm-update`}],runState:`running`,snapshots:[{name:`tm-leverage`,parts_in:200,parts_out:200},{name:`term-lookup`,parts_in:200,parts_out:200},{name:`ai-translate`,parts_in:200,parts_out:143},{name:`qa-check`,parts_in:143,parts_out:98},{name:`term-enforce`,parts_in:0,parts_out:0},{name:`tm-update`,parts_in:0,parts_out:0}]}},S={name:`Animated simulation`,render:()=>(0,s.jsx)(o,{steps:d,fileCount:3,partsPerFile:40})},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  name: "Idle (all pending)",
  args: {
    steps: twoSteps,
    runState: "idle"
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  name: "Running — all pending",
  args: {
    steps: fourSteps,
    runState: "running",
    snapshots: []
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "Running — first step active",
  args: {
    steps: fourSteps,
    runState: "running",
    snapshots: [{
      name: "tm-leverage",
      parts_in: 47,
      parts_out: 32
    }, {
      name: "ai-translate",
      parts_in: 0,
      parts_out: 0
    }, {
      name: "qa-check",
      parts_in: 0,
      parts_out: 0
    }, {
      name: "term-enforce",
      parts_in: 0,
      parts_out: 0
    }]
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "Running — mid-pipeline",
  args: {
    steps: fourSteps,
    runState: "running",
    snapshots: [{
      name: "tm-leverage",
      parts_in: 120,
      parts_out: 120
    }, {
      name: "ai-translate",
      parts_in: 120,
      parts_out: 87
    }, {
      name: "qa-check",
      parts_in: 87,
      parts_out: 52
    }, {
      name: "term-enforce",
      parts_in: 0,
      parts_out: 0
    }]
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "Running — near complete",
  args: {
    steps: twoSteps,
    runState: "running",
    snapshots: [{
      name: "ai-translate",
      parts_in: 120,
      parts_out: 120
    }, {
      name: "qa-check",
      parts_in: 120,
      parts_out: 118
    }]
  }
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "Complete",
  args: {
    steps: fourSteps,
    runState: "complete",
    snapshots: [{
      name: "tm-leverage",
      parts_in: 120,
      parts_out: 120
    }, {
      name: "ai-translate",
      parts_in: 120,
      parts_out: 120
    }, {
      name: "qa-check",
      parts_in: 120,
      parts_out: 120
    }, {
      name: "term-enforce",
      parts_in: 120,
      parts_out: 120
    }]
  }
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  name: "Error (frozen mid-run)",
  args: {
    steps: fourSteps,
    runState: "error",
    snapshots: [{
      name: "tm-leverage",
      parts_in: 120,
      parts_out: 120
    }, {
      name: "ai-translate",
      parts_in: 45,
      parts_out: 12
    }, {
      name: "qa-check",
      parts_in: 0,
      parts_out: 0
    }, {
      name: "term-enforce",
      parts_in: 0,
      parts_out: 0
    }]
  }
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  name: "Canceled",
  args: {
    steps: twoSteps,
    runState: "canceled",
    snapshots: [{
      name: "ai-translate",
      parts_in: 60,
      parts_out: 33
    }, {
      name: "qa-check",
      parts_in: 0,
      parts_out: 0
    }]
  }
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  name: "Single step",
  args: {
    steps: [{
      tool: "pseudo-translate"
    }],
    runState: "running",
    snapshots: [{
      name: "pseudo-translate",
      parts_in: 42,
      parts_out: 17
    }]
  }
}`,...b.parameters?.docs?.source}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  name: "Long pipeline (6 steps)",
  args: {
    steps: [{
      tool: "tm-leverage"
    }, {
      tool: "term-lookup"
    }, {
      tool: "ai-translate"
    }, {
      tool: "qa-check"
    }, {
      tool: "term-enforce"
    }, {
      tool: "tm-update"
    }],
    runState: "running",
    snapshots: [{
      name: "tm-leverage",
      parts_in: 200,
      parts_out: 200
    }, {
      name: "term-lookup",
      parts_in: 200,
      parts_out: 200
    }, {
      name: "ai-translate",
      parts_in: 200,
      parts_out: 143
    }, {
      name: "qa-check",
      parts_in: 143,
      parts_out: 98
    }, {
      name: "term-enforce",
      parts_in: 0,
      parts_out: 0
    }, {
      name: "tm-update",
      parts_in: 0,
      parts_out: 0
    }]
  }
}`,...x.parameters?.docs?.source}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  name: "Animated simulation",
  render: () => <AnimatedPipeline steps={fourSteps} fileCount={3} partsPerFile={40} />
}`,...S.parameters?.docs?.source}}},C=[`Idle`,`AllPending`,`FirstStepActive`,`MidPipeline`,`NearComplete`,`Complete`,`Error`,`Canceled`,`SingleStep`,`LongPipeline`,`Animated`]}))();export{p as AllPending,S as Animated,y as Canceled,_ as Complete,v as Error,m as FirstStepActive,f as Idle,x as LongPipeline,h as MidPipeline,g as NearComplete,b as SingleStep,C as __namedExportsOrder,l as default};