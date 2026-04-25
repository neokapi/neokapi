import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{i as o,n as s,r as c,t as l}from"./target-path-input-Dk07-U1j.js";function u({initial:e=``,placeholder:t}){let[n,r]=(0,f.useState)(e);return(0,p.jsxs)(`div`,{className:`max-w-lg space-y-2`,children:[(0,p.jsx)(c,{value:n,onChange:r,placeholder:t}),(0,p.jsx)(`pre`,{className:`max-h-[60px] overflow-auto rounded bg-muted p-2 font-mono text-xs`,children:JSON.stringify(n)})]})}function d({initial:e=``,placeholder:t}){let[n,r]=(0,f.useState)(e);return(0,p.jsxs)(`div`,{className:`max-w-lg space-y-2`,children:[(0,p.jsx)(l,{value:n,onChange:r,placeholder:t}),(0,p.jsx)(`pre`,{className:`max-h-[60px] overflow-auto rounded bg-muted p-2 font-mono text-xs`,children:JSON.stringify(n)})]})}var f,p,m,h,g,_,v,y,b,x,S,C,w;t((()=>{i(),f=e(n(),1),o(),s(),p=r(),m={title:`Foundations/GlobInput`,component:c,tags:[`autodocs`],parameters:{docs:{description:{component:`Single-line input with glob pattern syntax highlighting. Highlights **, *, ?, {braces}, [classes], and path separators.`}}}},h={name:`Glob — Basic`,render:()=>(0,p.jsx)(u,{initial:`src/locales/en/*.json`,placeholder:a(`35jmrS`,`src/**/*.json`)})},g={name:`Glob — Globstar`,render:()=>(0,p.jsx)(u,{initial:`docs/**/*.md`})},_={name:`Glob — Brace Expansion`,render:()=>(0,p.jsx)(u,{initial:`src/**/*.{ts,tsx,js,jsx}`})},v={name:`Glob — Character Class`,render:()=>(0,p.jsx)(u,{initial:`data/[0-9]*-report.csv`})},y={name:`Glob — Complex`,render:()=>(0,p.jsx)(u,{initial:`src/**/i18n/{en,fr}/**/*.{json,yaml}`})},b={name:`Target Path — Basic`,render:()=>(0,p.jsx)(d,{initial:`src/locales/{lang}/*.json`,placeholder:a(`4iOoE4`,`output/{lang}/**/*`)})},x={name:`Target Path — Multiple Variables`,render:()=>(0,p.jsx)(d,{initial:`output/{lang}/{region}/**/*.json`})},S={name:`Target Path — With Wildcards`,render:()=>(0,p.jsx)(d,{initial:`dist/{lang}/**/*`})},C={name:`Glob + Target Path — Side by Side`,render:()=>(0,p.jsxs)(`div`,{className:`max-w-2xl space-y-3`,children:[(0,p.jsxs)(`div`,{children:[(0,p.jsx)(`label`,{className:`mb-1 block text-xs text-muted-foreground`,children:a(`1YrdCW`,`Path pattern (glob)`)}),(0,p.jsx)(u,{initial:`src/i18n/en/**/*.json`})]}),(0,p.jsxs)(`div`,{children:[(0,p.jsx)(`label`,{className:`mb-1 block text-xs text-muted-foreground`,children:a(`2SG65`,`Target path`)}),(0,p.jsx)(d,{initial:`src/i18n/{lang}/**/*.json`})]})]})},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "Glob — Basic",
  render: () => <GlobWrapper initial="src/locales/en/*.json" placeholder="src/**/*.json" />
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "Glob — Globstar",
  render: () => <GlobWrapper initial="docs/**/*.md" />
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "Glob — Brace Expansion",
  render: () => <GlobWrapper initial="src/**/*.{ts,tsx,js,jsx}" />
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  name: "Glob — Character Class",
  render: () => <GlobWrapper initial="data/[0-9]*-report.csv" />
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  name: "Glob — Complex",
  render: () => <GlobWrapper initial="src/**/i18n/{en,fr}/**/*.{json,yaml}" />
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  name: "Target Path — Basic",
  render: () => <TargetWrapper initial="src/locales/{lang}/*.json" placeholder="output/{lang}/**/*" />
}`,...b.parameters?.docs?.source}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  name: "Target Path — Multiple Variables",
  render: () => <TargetWrapper initial="output/{lang}/{region}/**/*.json" />
}`,...x.parameters?.docs?.source}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  name: "Target Path — With Wildcards",
  render: () => <TargetWrapper initial="dist/{lang}/**/*" />
}`,...S.parameters?.docs?.source}}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  name: "Glob + Target Path — Side by Side",
  render: () => <div className="max-w-2xl space-y-3">
      <div>
        <label className="mb-1 block text-xs text-muted-foreground">Path pattern (glob)</label>
        <GlobWrapper initial="src/i18n/en/**/*.json" />
      </div>
      <div>
        <label className="mb-1 block text-xs text-muted-foreground">Target path</label>
        <TargetWrapper initial="src/i18n/{lang}/**/*.json" />
      </div>
    </div>
}`,...C.parameters?.docs?.source}}},w=[`BasicGlob`,`Globstar`,`BraceExpansion`,`CharacterClass`,`ComplexPattern`,`TargetPathBasic`,`TargetPathMultipleVars`,`TargetPathWithWildcards`,`SideBySide`]}))();export{h as BasicGlob,_ as BraceExpansion,v as CharacterClass,y as ComplexPattern,g as Globstar,C as SideBySide,b as TargetPathBasic,x as TargetPathMultipleVars,S as TargetPathWithWildcards,w as __namedExportsOrder,m as default};