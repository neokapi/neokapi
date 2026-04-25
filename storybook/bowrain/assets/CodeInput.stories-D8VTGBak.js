import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,t as a}from"./code-input-hDHqDx8d.js";function o({initial:e=``,language:t=`plain`,placeholder:n,singleLine:r,disabled:i,minHeight:o}){let[l,u]=(0,s.useState)(e);return(0,c.jsxs)(`div`,{className:`max-w-lg space-y-2`,children:[(0,c.jsx)(a,{value:l,onChange:u,language:t,placeholder:n,singleLine:r,disabled:i,minHeight:o}),(0,c.jsx)(`pre`,{className:`p-2 rounded bg-muted text-xs font-mono overflow-auto max-h-[100px]`,children:JSON.stringify(l)})]})}var s,c,l,u,d,f,p,m,h,g,_;t((()=>{s=e(n(),1),i(),c=r(),l={title:`Foundations/CodeInput`,component:a,tags:[`autodocs`],parameters:{docs:{description:{component:`CodeMirror 6 based code editor with syntax highlighting. Supports JavaScript, JSON, regex, and plain text modes. Used by SchemaForm code-editor widget and CodeFinderEditor regex inputs.`}}}},u={render:()=>(0,c.jsx)(o,{language:`javascript`,placeholder:`// Enter JavaScript code...`,initial:`function transform(segment) {
  const text = segment.target || segment.source;
  return text.toUpperCase();
}`,minHeight:160})},d={name:`JSON`,render:()=>(0,c.jsx)(o,{language:`json`,placeholder:`{}`,initial:`{
  "extractAll": true,
  "pathRules": "$.messages[*].text",
  "useCodeFinder": false
}`,minHeight:140})},f={render:()=>(0,c.jsx)(o,{language:`regex`,placeholder:`Regex pattern...`,initial:`</?\\\\w[^>]*>`,singleLine:!0})},p={name:`Regex — Multiple Patterns`,render:()=>(0,c.jsx)(`div`,{className:`max-w-lg space-y-1.5`,children:[`</?\\w[^>]*>`,`\\{\\d+\\}`,`%[-+0 #]*\\d*\\.?\\d*[diouxXeEfgGaAcspn%]`,`\\$\\{[^}]+\\}`].map((e,t)=>(0,c.jsxs)(`div`,{className:`flex items-center gap-2`,children:[(0,c.jsx)(`span`,{className:`text-xs text-muted-foreground w-4 text-right`,children:t+1}),(0,c.jsx)(o,{initial:e,language:`regex`,singleLine:!0})]},t))})},m={render:()=>(0,c.jsx)(o,{language:`plain`,placeholder:`Enter text...`,initial:`One rule per line\\nAnother rule here`,minHeight:100})},h={render:()=>(0,c.jsx)(o,{language:`javascript`,placeholder:`Single line expression...`,initial:`segment.source.replace(/\\\\s+/g, ' ')`,singleLine:!0})},g={render:()=>(0,c.jsx)(o,{language:`javascript`,initial:`const x = 42;`,disabled:!0,singleLine:!0})},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper language="javascript" placeholder="// Enter JavaScript code..." initial={\`function transform(segment) {\\n  const text = segment.target || segment.source;\\n  return text.toUpperCase();\\n}\`} minHeight={160} />
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  name: "JSON",
  render: () => <Wrapper language="json" placeholder="{}" initial={\`{\\n  "extractAll": true,\\n  "pathRules": "$.messages[*].text",\\n  "useCodeFinder": false\\n}\`} minHeight={140} />
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper language="regex" placeholder="Regex pattern..." initial="</?\\\\w[^>]*>" singleLine />
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
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
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper language="plain" placeholder="Enter text..." initial="One rule per line\\nAnother rule here" minHeight={100} />
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper language="javascript" placeholder="Single line expression..." initial="segment.source.replace(/\\\\s+/g, ' ')" singleLine />
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper language="javascript" initial="const x = 42;" disabled singleLine />
}`,...g.parameters?.docs?.source}}},_=[`JavaScript`,`JsonMode`,`Regex`,`RegexMultiplePatterns`,`PlainText`,`SingleLine`,`Disabled`]}))();export{g as Disabled,u as JavaScript,d as JsonMode,m as PlainText,f as Regex,p as RegexMultiplePatterns,h as SingleLine,_ as __namedExportsOrder,l as default};