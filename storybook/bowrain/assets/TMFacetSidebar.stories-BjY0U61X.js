import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,r as a,t as o}from"./TMFacetSidebar-BUB5Sjt9.js";var s,c,l,u,d,f,p,m,h,g;t((()=>{s=e(n(),1),a(),c=r(),l=new Date().toISOString(),u={locales:[{locale:`en-US`,count:128},{locale:`fr-FR`,count:42},{locale:`de-DE`,count:38},{locale:`ja-JP`,count:25},{locale:`es-ES`,count:15},{locale:`zh-CN`,count:8}],projects:[{project_id:`webapp`,count:80},{project_id:`mobile`,count:35},{project_id:``,count:13}],entity_types:[{type:`entity:person`,count:12},{type:`entity:organization`,count:7},{type:`entity:product`,count:5},{type:`entity:date`,count:3}],import_sessions:[{session_id:`sess-1`,file_key:`acme-glossary.tmx`,tool_name:`tmx-import`,imported_at:new Date(Date.now()-1440*60*1e3).toISOString(),count:125},{session_id:`sess-2`,file_key:`legacy-memory.tmx`,tool_name:`tmx-import`,imported_at:l,count:48}],has_codes:45,no_codes:83},d={title:`Resource Browser/TMFacetSidebar`,component:i,tags:[`autodocs`],decorators:[e=>(0,c.jsx)(`div`,{style:{width:240,padding:16,borderLeft:`1px solid var(--border)`},children:(0,c.jsx)(e,{})})],parameters:{docs:{description:{component:`Left sidebar showing faceted filters for the TM browser. Sections: Languages, Project, Entity Types, Import Sessions, Inline Codes.`}}}},f={args:{facets:u,selection:o}},p={args:{facets:u,selection:{locales:[`fr-FR`,`de-DE`],projects:[`webapp`],entityTypes:[],sessionIds:[`sess-1`],codeFilter:`all`}}},m={render:()=>{let[e,t]=(0,s.useState)(o);return(0,c.jsxs)(`div`,{children:[(0,c.jsx)(i,{facets:u,selection:e,onSelectionChange:t}),(0,c.jsx)(`pre`,{className:`mt-4 text-[10px] text-muted-foreground`,children:JSON.stringify(e,null,2)})]})}},h={args:{facets:null,selection:o}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    facets: SAMPLE_FACETS,
    selection: EMPTY_FACETS
  }
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    facets: SAMPLE_FACETS,
    selection: {
      locales: ["fr-FR", "de-DE"],
      projects: ["webapp"],
      entityTypes: [],
      sessionIds: ["sess-1"],
      codeFilter: "all"
    }
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [selection, setSelection] = useState<FacetSelection>(EMPTY_FACETS);
    return <div>
        <TMFacetSidebar facets={SAMPLE_FACETS} selection={selection} onSelectionChange={setSelection} />
        <pre className="mt-4 text-[10px] text-muted-foreground">
          {JSON.stringify(selection, null, 2)}
        </pre>
      </div>;
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    facets: null,
    selection: EMPTY_FACETS
  }
}`,...h.parameters?.docs?.source}}},g=[`Default`,`WithActiveFilters`,`Interactive`,`Empty`]}))();export{f as Default,h as Empty,m as Interactive,p as WithActiveFilters,g as __namedExportsOrder,d as default};