import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,r,t as i}from"./runtime-B587fM_0.js";import{n as a,r as o,t as s}from"./DocsPanel-CvGvVb9s.js";import{n as c,t as l}from"./plugin-docs-n-ivgD6K.js";var u,d,f,p,m,h,g,_,v,y,b,x,S,C,w,T,E,D;e((()=>{r(),o(),l(),u=t(),d=c,f={title:`Components/DocsPanel`,component:s,tags:[`autodocs`],parameters:{layout:`padded`},decorators:[e=>(0,u.jsx)(`div`,{style:{maxWidth:640},children:(0,u.jsx)(e,{})})]},p={name:`JSON Filter`,args:{doc:d.filters.okf_json}},m={name:`HTML Filter`,args:{doc:d.filters.okf_html}},h={name:`XLIFF Filter`,args:{doc:d.filters.okf_xliff}},g={name:`Properties Filter`,args:{doc:d.filters.okf_properties}},_={name:`PO (Gettext) Filter`,args:{doc:d.filters.okf_po}},v=Object.values(d.steps)[0],y=Object.values(d.steps)[1],b={name:`Pipeline Step`,args:{doc:v}},x={name:`Second Step`,args:{doc:y}},S={name:`Inline (no card wrapper)`,args:{doc:d.filters.okf_json,inline:!0}},C={name:`Filtered Parameters (extraction only)`,args:{doc:d.filters.okf_json,visibleParams:[`extraction`]}},w={name:`ParamHelp (inline tooltip)`,render:()=>(0,u.jsxs)(`div`,{className:`space-y-3 p-4`,children:[(0,u.jsx)(`p`,{className:`text-sm text-foreground`,children:i(`4DQHps`,`Click the info icons to see parameter help:`)}),(0,u.jsx)(`div`,{className:`flex items-center gap-2 text-sm`,children:n(`sz4vL`,`{=m0} {=m1}`,{"=m0":(0,u.jsx)(`span`,{className:`text-foreground`,children:`inlineCodes`}),"=m1":(0,u.jsx)(a,{paramKey:`inlineCodes`,doc:d.filters.okf_json})})}),(0,u.jsx)(`div`,{className:`flex items-center gap-2 text-sm`,children:n(`sz4vL`,`{=m0} {=m1}`,{"=m0":(0,u.jsx)(`span`,{className:`text-foreground`,children:`extraction.extractAll`}),"=m1":(0,u.jsx)(a,{paramKey:`extraction.extractAll`,doc:d.filters.okf_json})})}),(0,u.jsx)(`div`,{className:`flex items-center gap-2 text-sm`,children:n(`sz4vL`,`{=m0} {=m1}`,{"=m0":(0,u.jsx)(`span`,{className:`text-foreground`,children:`nonexistent (no tooltip)`}),"=m1":(0,u.jsx)(a,{paramKey:`nonexistent`,doc:d.filters.okf_json})})})]})},T={name:`All Filters Gallery`,render:()=>(0,u.jsx)(`div`,{className:`space-y-6`,children:Object.entries(d.filters).map(([e,t])=>(0,u.jsxs)(`div`,{children:[(0,u.jsx)(`h3`,{className:`text-sm font-semibold text-foreground mb-2`,children:e}),(0,u.jsx)(s,{doc:t})]},e))})},E={name:`All Steps Gallery`,render:()=>(0,u.jsx)(`div`,{className:`space-y-6`,children:Object.entries(d.steps).map(([e,t])=>(0,u.jsxs)(`div`,{children:[(0,u.jsx)(`h3`,{className:`text-sm font-semibold text-foreground mb-2`,children:e}),(0,u.jsx)(s,{doc:t})]},e))})},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  name: "JSON Filter",
  args: {
    doc: docs.filters.okf_json as FilterDoc
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "HTML Filter",
  args: {
    doc: docs.filters.okf_html as FilterDoc
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "XLIFF Filter",
  args: {
    doc: docs.filters.okf_xliff as FilterDoc
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "Properties Filter",
  args: {
    doc: docs.filters.okf_properties as FilterDoc
  }
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "PO (Gettext) Filter",
  args: {
    doc: docs.filters.okf_po as FilterDoc
  }
}`,..._.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  name: "Pipeline Step",
  args: {
    doc: firstStep
  }
}`,...b.parameters?.docs?.source}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  name: "Second Step",
  args: {
    doc: secondStep
  }
}`,...x.parameters?.docs?.source}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  name: "Inline (no card wrapper)",
  args: {
    doc: docs.filters.okf_json as FilterDoc,
    inline: true
  }
}`,...S.parameters?.docs?.source}}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  name: "Filtered Parameters (extraction only)",
  args: {
    doc: docs.filters.okf_json as FilterDoc,
    visibleParams: ["extraction"]
  }
}`,...C.parameters?.docs?.source}}},w.parameters={...w.parameters,docs:{...w.parameters?.docs,source:{originalSource:`{
  name: "ParamHelp (inline tooltip)",
  render: () => <div className="space-y-3 p-4">
      <p className="text-sm text-foreground">Click the info icons to see parameter help:</p>
      <div className="flex items-center gap-2 text-sm">
        <span className="text-foreground">inlineCodes</span>
        <ParamHelp paramKey="inlineCodes" doc={docs.filters.okf_json as FilterDoc} />
      </div>
      <div className="flex items-center gap-2 text-sm">
        <span className="text-foreground">extraction.extractAll</span>
        <ParamHelp paramKey="extraction.extractAll" doc={docs.filters.okf_json as FilterDoc} />
      </div>
      <div className="flex items-center gap-2 text-sm">
        <span className="text-foreground">nonexistent (no tooltip)</span>
        <ParamHelp paramKey="nonexistent" doc={docs.filters.okf_json as FilterDoc} />
      </div>
    </div>
}`,...w.parameters?.docs?.source}}},T.parameters={...T.parameters,docs:{...T.parameters?.docs,source:{originalSource:`{
  name: "All Filters Gallery",
  render: () => <div className="space-y-6">
      {Object.entries(docs.filters).map(([id, filterDoc]) => <div key={id}>
          <h3 className="text-sm font-semibold text-foreground mb-2">{id}</h3>
          <DocsPanel doc={filterDoc as FilterDoc} />
        </div>)}
    </div>
}`,...T.parameters?.docs?.source}}},E.parameters={...E.parameters,docs:{...E.parameters?.docs,source:{originalSource:`{
  name: "All Steps Gallery",
  render: () => <div className="space-y-6">
      {Object.entries(docs.steps).map(([id, stepDoc]) => <div key={id}>
          <h3 className="text-sm font-semibold text-foreground mb-2">{id}</h3>
          <DocsPanel doc={stepDoc as StepDoc} />
        </div>)}
    </div>
}`,...E.parameters?.docs?.source}}},D=[`JSONFilter`,`HTMLFilter`,`XLIFFFilter`,`PropertiesFilter`,`POFilter`,`PipelineStep`,`SecondStep`,`InlineMode`,`FilteredParams`,`ParameterHelp`,`FilterGallery`,`StepGallery`]}))();export{T as FilterGallery,C as FilteredParams,m as HTMLFilter,S as InlineMode,p as JSONFilter,_ as POFilter,w as ParameterHelp,b as PipelineStep,g as PropertiesFilter,x as SecondStep,E as StepGallery,h as XLIFFFilter,D as __namedExportsOrder,f as default};