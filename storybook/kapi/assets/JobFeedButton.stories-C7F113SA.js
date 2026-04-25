import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,r as a,t as o}from"./runtime-B587fM_0.js";import{St as s,V as c,r as l,t as u,u as d,xt as f,z as p}from"./lucide-react-CNTu2eXS.js";import{t as m}from"./src-B5GKo4pn.js";import{t as h}from"./button-Dd5Xmast.js";import{i as g}from"./TMFacetSidebar-BYYSd6v4.js";function _({initialJobs:e=[],simulateProgress:t=!1}){let[n,r]=(0,v.useState)(e),[a,u]=(0,v.useState)(!0);(0,v.useEffect)(()=>{if(!t)return;let e=setInterval(()=>{r(e=>e.map(e=>{if(e.status!==`running`)return e;let t=e.progress.current+1;return t>=e.progress.total?{...e,status:`complete`,durationMs:4200,progress:{...e.progress,current:e.progress.total}}:{...e,progress:{...e.progress,current:t}}}))},800);return()=>clearInterval(e)},[t]);let m=n.some(e=>e.status===`running`),_=e=>{switch(e.status){case`running`:return(0,y.jsx)(p,{size:13,className:`animate-spin text-primary shrink-0`});case`complete`:return(0,y.jsx)(s,{size:13,className:`text-green-500 shrink-0`});case`error`:return(0,y.jsx)(f,{size:13,className:`text-destructive shrink-0`})}};return(0,y.jsxs)(`div`,{className:`relative inline-block`,children:[(0,y.jsxs)(h,{variant:`ghost`,size:`icon-sm`,onClick:()=>u(e=>!e),className:`relative h-7 w-7`,children:[m?(0,y.jsx)(p,{size:15,className:`animate-spin text-primary`}):(0,y.jsx)(c,{size:15,className:`text-muted-foreground`}),n.length>0&&(0,y.jsx)(`span`,{className:`absolute -top-0.5 -right-0.5 flex h-3.5 w-3.5 items-center justify-center rounded-full bg-primary text-[8px] font-bold text-primary-foreground`,children:n.length})]}),a&&(0,y.jsxs)(`div`,{className:`absolute left-0 top-full mt-1 z-50 w-72 rounded-lg border border-border bg-card shadow-lg overflow-hidden`,children:[(0,y.jsxs)(`div`,{className:`flex items-center justify-between px-3 py-2 border-b border-border bg-muted/30`,children:[(0,y.jsx)(`span`,{className:`text-[11px] font-semibold text-foreground`,children:o(`2ArptX`,`Jobs`)}),(0,y.jsxs)(`div`,{className:`flex items-center gap-1`,children:[(0,y.jsx)(`button`,{className:`text-[10px] text-muted-foreground hover:text-foreground`,children:(0,y.jsx)(d,{size:11})}),(0,y.jsx)(`button`,{onClick:()=>u(!1),className:`p-0.5 rounded hover:bg-muted/60`,children:(0,y.jsx)(l,{size:12,className:`text-muted-foreground`})})]})]}),n.length===0?(0,y.jsx)(`div`,{className:`px-3 py-6 text-center text-xs text-muted-foreground`,children:o(`1W2DB8`,`No recent jobs`)}):(0,y.jsx)(g,{className:`max-h-64`,children:(0,y.jsx)(`div`,{className:`divide-y divide-border/30`,children:n.map(e=>(0,y.jsxs)(`div`,{className:`flex items-start gap-2 px-3 py-2 hover:bg-muted/30 cursor-pointer group`,children:[(0,y.jsx)(`div`,{className:`mt-0.5`,children:_(e)}),(0,y.jsxs)(`div`,{className:`flex-1 min-w-0`,children:[(0,y.jsx)(`div`,{className:`text-xs font-medium text-foreground truncate`,children:e.flowName}),e.status===`running`&&e.progress.total>0&&(0,y.jsxs)(`div`,{className:`mt-1`,children:[(0,y.jsx)(`div`,{className:`flex justify-between text-[10px] text-muted-foreground mb-0.5`,children:i(`29yEaK`,`{=m0} {=m1}`,{"=m0":(0,y.jsxs)(`span`,{children:[e.progress.current,`/`,e.progress.total]}),"=m1":(0,y.jsxs)(`span`,{children:[Math.round(e.progress.current/e.progress.total*100),`%`]})})}),(0,y.jsx)(`div`,{className:`h-1 rounded-full bg-accent overflow-hidden`,children:(0,y.jsx)(`div`,{className:`h-full rounded-full bg-primary transition-all duration-300`,style:{width:`${e.progress.current/e.progress.total*100}%`}})})]}),e.status===`complete`&&(0,y.jsx)(`div`,{className:`text-[10px] text-muted-foreground mt-0.5`,children:e.durationMs?`${(e.durationMs/1e3).toFixed(1)}s`:``}),e.status===`error`&&(0,y.jsx)(`div`,{className:`text-[10px] text-destructive mt-0.5 truncate`,children:e.error})]})]},e.id))})})]})]})}var v,y,b,x,S,C,w,T,E;t((()=>{a(),v=e(n(),1),u(),m(),y=r(),b={title:`App/JobFeedButton`,component:_,parameters:{layout:`padded`,docs:{description:{component:`Persistent job feed button with dropdown panel. Shows active, completed, and errored flow executions.`}}}},x={args:{initialJobs:[]}},S={name:`Running job with progress`,args:{initialJobs:[{id:`1`,flowName:`translate`,status:`running`,progress:{current:5,total:16}}],simulateProgress:!0}},C={args:{initialJobs:[{id:`1`,flowName:`translate`,status:`complete`,progress:{current:16,total:16},durationMs:12400}]}},w={args:{initialJobs:[{id:`1`,flowName:`translate-and-qa`,status:`error`,progress:{current:3,total:16},error:`API rate limit exceeded`}]}},T={name:`Multiple jobs (mixed status)`,args:{initialJobs:[{id:`1`,flowName:`translate (de-DE)`,status:`running`,progress:{current:8,total:16}},{id:`2`,flowName:`translate (fr-FR)`,status:`complete`,progress:{current:16,total:16},durationMs:8200},{id:`3`,flowName:`qa-check`,status:`error`,progress:{current:2,total:10},error:`Provider unavailable`}],simulateProgress:!0}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  args: {
    initialJobs: []
  }
}`,...x.parameters?.docs?.source}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  name: "Running job with progress",
  args: {
    initialJobs: [{
      id: "1",
      flowName: "translate",
      status: "running",
      progress: {
        current: 5,
        total: 16
      }
    }],
    simulateProgress: true
  }
}`,...S.parameters?.docs?.source}}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  args: {
    initialJobs: [{
      id: "1",
      flowName: "translate",
      status: "complete",
      progress: {
        current: 16,
        total: 16
      },
      durationMs: 12400
    }]
  }
}`,...C.parameters?.docs?.source}}},w.parameters={...w.parameters,docs:{...w.parameters?.docs,source:{originalSource:`{
  args: {
    initialJobs: [{
      id: "1",
      flowName: "translate-and-qa",
      status: "error",
      progress: {
        current: 3,
        total: 16
      },
      error: "API rate limit exceeded"
    }]
  }
}`,...w.parameters?.docs?.source}}},T.parameters={...T.parameters,docs:{...T.parameters?.docs,source:{originalSource:`{
  name: "Multiple jobs (mixed status)",
  args: {
    initialJobs: [{
      id: "1",
      flowName: "translate (de-DE)",
      status: "running",
      progress: {
        current: 8,
        total: 16
      }
    }, {
      id: "2",
      flowName: "translate (fr-FR)",
      status: "complete",
      progress: {
        current: 16,
        total: 16
      },
      durationMs: 8200
    }, {
      id: "3",
      flowName: "qa-check",
      status: "error",
      progress: {
        current: 2,
        total: 10
      },
      error: "Provider unavailable"
    }],
    simulateProgress: true
  }
}`,...T.parameters?.docs?.source}}},E=[`Empty`,`RunningJob`,`CompletedJob`,`ErroredJob`,`MultipleJobs`]}))();export{C as CompletedJob,x as Empty,w as ErroredJob,T as MultipleJobs,S as RunningJob,E as __namedExportsOrder,b as default};