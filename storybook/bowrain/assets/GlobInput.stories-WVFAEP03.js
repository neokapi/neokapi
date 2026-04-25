import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{i,n as a,r as o,t as s}from"./target-path-input-BNN9DrXV.js";function c({initial:e=``,placeholder:t}){let[n,r]=(0,u.useState)(e);return(0,d.jsxs)(`div`,{className:`max-w-lg space-y-2`,children:[(0,d.jsx)(o,{value:n,onChange:r,placeholder:t}),(0,d.jsx)(`pre`,{className:`max-h-[60px] overflow-auto rounded bg-muted p-2 font-mono text-xs`,children:JSON.stringify(n)})]})}function l({initial:e=``,placeholder:t}){let[n,r]=(0,u.useState)(e);return(0,d.jsxs)(`div`,{className:`max-w-lg space-y-2`,children:[(0,d.jsx)(s,{value:n,onChange:r,placeholder:t}),(0,d.jsx)(`pre`,{className:`max-h-[60px] overflow-auto rounded bg-muted p-2 font-mono text-xs`,children:JSON.stringify(n)})]})}var u,d,f,p,m,h,g,_,v,y,b,x,S;t((()=>{u=e(n(),1),i(),a(),d=r(),f={title:`Foundations/GlobInput`,component:o,tags:[`autodocs`],parameters:{docs:{description:{component:`Single-line input with glob pattern syntax highlighting. Highlights **, *, ?, {braces}, [classes], and path separators.`}}}},p={name:`Glob — Basic`,render:()=>(0,d.jsx)(c,{initial:`src/locales/en/*.json`,placeholder:`src/**/*.json`})},m={name:`Glob — Globstar`,render:()=>(0,d.jsx)(c,{initial:`docs/**/*.md`})},h={name:`Glob — Brace Expansion`,render:()=>(0,d.jsx)(c,{initial:`src/**/*.{ts,tsx,js,jsx}`})},g={name:`Glob — Character Class`,render:()=>(0,d.jsx)(c,{initial:`data/[0-9]*-report.csv`})},_={name:`Glob — Complex`,render:()=>(0,d.jsx)(c,{initial:`src/**/i18n/{en,fr}/**/*.{json,yaml}`})},v={name:`Target Path — Basic`,render:()=>(0,d.jsx)(l,{initial:`src/locales/{lang}/*.json`,placeholder:`output/{lang}/**/*`})},y={name:`Target Path — Multiple Variables`,render:()=>(0,d.jsx)(l,{initial:`output/{lang}/{region}/**/*.json`})},b={name:`Target Path — With Wildcards`,render:()=>(0,d.jsx)(l,{initial:`dist/{lang}/**/*`})},x={name:`Glob + Target Path — Side by Side`,render:()=>(0,d.jsxs)(`div`,{className:`max-w-2xl space-y-3`,children:[(0,d.jsxs)(`div`,{children:[(0,d.jsx)(`label`,{className:`mb-1 block text-xs text-muted-foreground`,children:`Path pattern (glob)`}),(0,d.jsx)(c,{initial:`src/i18n/en/**/*.json`})]}),(0,d.jsxs)(`div`,{children:[(0,d.jsx)(`label`,{className:`mb-1 block text-xs text-muted-foreground`,children:`Target path`}),(0,d.jsx)(l,{initial:`src/i18n/{lang}/**/*.json`})]})]})},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  name: "Glob — Basic",
  render: () => <GlobWrapper initial="src/locales/en/*.json" placeholder="src/**/*.json" />
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "Glob — Globstar",
  render: () => <GlobWrapper initial="docs/**/*.md" />
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "Glob — Brace Expansion",
  render: () => <GlobWrapper initial="src/**/*.{ts,tsx,js,jsx}" />
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "Glob — Character Class",
  render: () => <GlobWrapper initial="data/[0-9]*-report.csv" />
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "Glob — Complex",
  render: () => <GlobWrapper initial="src/**/i18n/{en,fr}/**/*.{json,yaml}" />
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  name: "Target Path — Basic",
  render: () => <TargetWrapper initial="src/locales/{lang}/*.json" placeholder="output/{lang}/**/*" />
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  name: "Target Path — Multiple Variables",
  render: () => <TargetWrapper initial="output/{lang}/{region}/**/*.json" />
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  name: "Target Path — With Wildcards",
  render: () => <TargetWrapper initial="dist/{lang}/**/*" />
}`,...b.parameters?.docs?.source}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
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
}`,...x.parameters?.docs?.source}}},S=[`BasicGlob`,`Globstar`,`BraceExpansion`,`CharacterClass`,`ComplexPattern`,`TargetPathBasic`,`TargetPathMultipleVars`,`TargetPathWithWildcards`,`SideBySide`]}))();export{p as BasicGlob,h as BraceExpansion,g as CharacterClass,_ as ComplexPattern,m as Globstar,x as SideBySide,v as TargetPathBasic,y as TargetPathMultipleVars,b as TargetPathWithWildcards,S as __namedExportsOrder,f as default};