import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{n as o,t as s}from"./code-input-C79deU_K.js";function c({initial:e=``,language:t=`plain`,placeholder:n,singleLine:r,disabled:i,minHeight:a}){let[o,c]=(0,l.useState)(e);return(0,u.jsxs)(`div`,{className:`max-w-lg space-y-2`,children:[(0,u.jsx)(s,{value:o,onChange:c,language:t,placeholder:n,singleLine:r,disabled:i,minHeight:a}),(0,u.jsx)(`pre`,{className:`p-2 rounded bg-muted text-xs font-mono overflow-auto max-h-[100px]`,children:JSON.stringify(o)})]})}var l,u,d,f,p,m,h,g,_,v,y;t((()=>{i(),l=e(n(),1),o(),u=r(),d={title:`Foundations/CodeInput`,component:s,tags:[`autodocs`],parameters:{docs:{description:{component:`CodeMirror 6 based code editor with syntax highlighting. Supports JavaScript, JSON, regex, and plain text modes. Used by SchemaForm code-editor widget and CodeFinderEditor regex inputs.`}}}},f={render:()=>(0,u.jsx)(c,{language:`javascript`,placeholder:a(`32nDPr`,`// Enter JavaScript code...`),initial:`function transform(segment) {
  const text = segment.target || segment.source;
  return text.toUpperCase();
}`,minHeight:160})},p={name:`JSON`,render:()=>(0,u.jsx)(c,{language:`json`,placeholder:a(`3LoLdX`,`{}`),initial:`{
  "extractAll": true,
  "pathRules": "$.messages[*].text",
  "useCodeFinder": false
}`,minHeight:140})},m={render:()=>(0,u.jsx)(c,{language:`regex`,placeholder:a(`370bkp`,`Regex pattern...`),initial:`</?\\\\w[^>]*>`,singleLine:!0})},h={name:`Regex — Multiple Patterns`,render:()=>(0,u.jsx)(`div`,{className:`max-w-lg space-y-1.5`,children:[`</?\\w[^>]*>`,`\\{\\d+\\}`,`%[-+0 #]*\\d*\\.?\\d*[diouxXeEfgGaAcspn%]`,`\\$\\{[^}]+\\}`].map((e,t)=>(0,u.jsxs)(`div`,{className:`flex items-center gap-2`,children:[(0,u.jsx)(`span`,{className:`text-xs text-muted-foreground w-4 text-right`,children:t+1}),(0,u.jsx)(c,{initial:e,language:`regex`,singleLine:!0})]},t))})},g={render:()=>(0,u.jsx)(c,{language:`plain`,placeholder:a(`Mel0X`,`Enter text...`),initial:`One rule per line\\nAnother rule here`,minHeight:100})},_={render:()=>(0,u.jsx)(c,{language:`javascript`,placeholder:a(`zaaKX`,`Single line expression...`),initial:`segment.source.replace(/\\\\s+/g, ' ')`,singleLine:!0})},v={render:()=>(0,u.jsx)(c,{language:`javascript`,initial:`const x = 42;`,disabled:!0,singleLine:!0})},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper language="javascript" placeholder="// Enter JavaScript code..." initial={\`function transform(segment) {\\n  const text = segment.target || segment.source;\\n  return text.toUpperCase();\\n}\`} minHeight={160} />
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  name: "JSON",
  render: () => <Wrapper language="json" placeholder="{}" initial={\`{\\n  "extractAll": true,\\n  "pathRules": "$.messages[*].text",\\n  "useCodeFinder": false\\n}\`} minHeight={140} />
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper language="regex" placeholder="Regex pattern..." initial="</?\\\\w[^>]*>" singleLine />
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "Regex — Multiple Patterns",
  render: () => {
    const patterns = ["</?\\\\w[^>]*>", "\\\\{\\\\d+\\\\}", "%[-+0 #]*\\\\d*\\\\.?\\\\d*[diouxXeEfgGaAcspn%]", "\\\\$\\\\{[^}]+\\\\}"];
    return <div className="max-w-lg space-y-1.5">
        {patterns.map((p, i) => <div key={i} className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-4 text-right">{i + 1}</span>
            <Wrapper initial={p} language="regex" singleLine />
          </div>)}
      </div>;
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper language="plain" placeholder="Enter text..." initial="One rule per line\\nAnother rule here" minHeight={100} />
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper language="javascript" placeholder="Single line expression..." initial="segment.source.replace(/\\\\s+/g, ' ')" singleLine />
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper language="javascript" initial="const x = 42;" disabled singleLine />
}`,...v.parameters?.docs?.source}}},y=[`JavaScript`,`JsonMode`,`Regex`,`RegexMultiplePatterns`,`PlainText`,`SingleLine`,`Disabled`]}))();export{v as Disabled,f as JavaScript,p as JsonMode,g as PlainText,m as Regex,h as RegexMultiplePatterns,_ as SingleLine,y as __namedExportsOrder,d as default};