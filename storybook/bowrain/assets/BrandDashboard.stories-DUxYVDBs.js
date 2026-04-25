import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{ct as n,st as r}from"./iframe-BJVhYAGS.js";import{d as i,f as a,i as o,o as s,p as c,r as l}from"./fixtures-Cf-tGaH5.js";var u,d,f,p,m,h,g,_;e((()=>{n(),o(),u=t(),d={title:`Brand/BrandDashboard`,component:r,tags:[`autodocs`],decorators:[e=>(0,u.jsx)(`div`,{style:{maxWidth:960,padding:24},children:(0,u.jsx)(e,{})})]},f={args:{score:null,trends:[],recentScores:[]}},p={args:{score:i,trends:c,recentScores:a}},m={args:{score:l,trends:c.map(e=>({...e,avg_score:e.avg_score+20})),recentScores:a.slice(0,2)}},h={args:{score:s,trends:c.map(e=>({...e,avg_score:Math.max(10,e.avg_score-30)})),recentScores:a}},g={args:{score:i,trends:[],recentScores:a}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    score: null,
    trends: [],
    recentScores: []
  }
}`,...f.parameters?.docs?.source},description:{story:`No compliance data yet.`,...f.parameters?.docs?.description}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    score: sampleScore,
    trends: sampleTrends,
    recentScores: sampleStoredScores
  }
}`,...p.parameters?.docs?.source},description:{story:`Typical mixed-score dashboard.`,...p.parameters?.docs?.description}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    score: excellentScore,
    trends: sampleTrends.map(t => ({
      ...t,
      avg_score: t.avg_score + 20
    })),
    recentScores: sampleStoredScores.slice(0, 2)
  }
}`,...m.parameters?.docs?.source},description:{story:`Excellent compliance across the board.`,...m.parameters?.docs?.description}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    score: poorScore,
    trends: sampleTrends.map(t => ({
      ...t,
      avg_score: Math.max(10, t.avg_score - 30)
    })),
    recentScores: sampleStoredScores
  }
}`,...h.parameters?.docs?.source},description:{story:`Poor compliance with many findings.`,...h.parameters?.docs?.description}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    score: sampleScore,
    trends: [],
    recentScores: sampleStoredScores
  }
}`,...g.parameters?.docs?.source},description:{story:`Data but no trend history yet.`,...g.parameters?.docs?.description}}},_=[`Empty`,`WithData`,`Excellent`,`Poor`,`NoTrends`]}))();export{f as Empty,m as Excellent,g as NoTrends,h as Poor,p as WithData,_ as __namedExportsOrder,d as default};