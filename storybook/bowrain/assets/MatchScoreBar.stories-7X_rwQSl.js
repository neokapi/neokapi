import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./MatchScoreBar-BtzhLzPA.js";var i,a,o,s,c,l,u,d,f,p;e((()=>{n(),i=t(),a={title:`Resource Browser/MatchScoreBar`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{width:300,padding:16},children:(0,i.jsx)(e,{})})],parameters:{docs:{description:{component:`Horizontal bar visualizing a TM match score (0-1.0). Color coded: red < 0.7, amber 0.7-0.85, green 0.85-0.99, blue 1.0.`}}}},o={args:{score:1,matchType:`exact`}},s={args:{score:.92,matchType:`fuzzy`}},c={args:{score:.75,matchType:`fuzzy`}},l={args:{score:.55,matchType:`fuzzy`}},u={args:{score:1,matchType:`generalized-exact`}},d={args:{score:.88,matchType:`structural-fuzzy`}},f={render:()=>(0,i.jsxs)(`div`,{className:`space-y-3`,children:[(0,i.jsx)(r,{score:1,matchType:`exact`}),(0,i.jsx)(r,{score:1,matchType:`generalized-exact`}),(0,i.jsx)(r,{score:.92,matchType:`fuzzy`}),(0,i.jsx)(r,{score:.88,matchType:`structural-fuzzy`}),(0,i.jsx)(r,{score:.75,matchType:`fuzzy`}),(0,i.jsx)(r,{score:.55,matchType:`fuzzy`})]})},o.parameters={...o.parameters,docs:{...o.parameters?.docs,source:{originalSource:`{
  args: {
    score: 1.0,
    matchType: "exact"
  }
}`,...o.parameters?.docs?.source}}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    score: 0.92,
    matchType: "fuzzy"
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    score: 0.75,
    matchType: "fuzzy"
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    score: 0.55,
    matchType: "fuzzy"
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    score: 1.0,
    matchType: "generalized-exact"
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    score: 0.88,
    matchType: "structural-fuzzy"
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => <div className="space-y-3">
      <MatchScoreBar score={1.0} matchType="exact" />
      <MatchScoreBar score={1.0} matchType="generalized-exact" />
      <MatchScoreBar score={0.92} matchType="fuzzy" />
      <MatchScoreBar score={0.88} matchType="structural-fuzzy" />
      <MatchScoreBar score={0.75} matchType="fuzzy" />
      <MatchScoreBar score={0.55} matchType="fuzzy" />
    </div>
}`,...f.parameters?.docs?.source},description:{story:`All match quality levels compared side by side.`,...f.parameters?.docs?.description}}},p=[`ExactMatch`,`HighFuzzy`,`MediumFuzzy`,`LowFuzzy`,`GeneralizedExact`,`StructuralFuzzy`,`AllScores`]}))();export{f as AllScores,o as ExactMatch,u as GeneralizedExact,s as HighFuzzy,l as LowFuzzy,c as MediumFuzzy,d as StructuralFuzzy,p as __namedExportsOrder,a as default};