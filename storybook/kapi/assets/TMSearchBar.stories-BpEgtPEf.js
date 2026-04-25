import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{n as o,t as s}from"./TMSearchBar-2B6EBo84.js";var c,l,u,d,f,p,m,h,g;t((()=>{i(),c=e(n(),1),o(),l=r(),u={title:`Resource Browser/TMSearchBar`,component:s,tags:[`autodocs`],decorators:[e=>(0,l.jsx)(`div`,{style:{maxWidth:600,padding:16},children:(0,l.jsx)(e,{})})],parameters:{docs:{description:{component:`Combined search bar with inline entity annotation. Select text to mark entities; press Enter to trigger fuzzy TM lookup.`}}}},d={render:()=>{let[e,t]=(0,c.useState)(``);return(0,l.jsx)(s,{value:e,onChange:t,sourceLocale:`en-US`,targetLocale:`fr-FR`})}},f={render:()=>{let[e,t]=(0,c.useState)(``),[n,r]=(0,c.useState)([{key:`language`,value:`fr-FR`},{key:`project`,value:`webapp`}]);return(0,l.jsx)(s,{value:e,onChange:t,filters:n,onFiltersChange:r,filterFields:[{key:`language`,label:`Target Language`,values:[{value:`fr-FR`,label:`French (fr-FR)`},{value:`de-DE`,label:`German (de-DE)`},{value:`ja-JP`,label:`Japanese (ja-JP)`}]},{key:`project`,label:`Project`,values:[{value:`webapp`,label:`Web App`},{value:`mobile`,label:`Mobile`}]}],sourceLocale:`en-US`,targetLocale:`fr-FR`})},parameters:{docs:{description:{story:`Filter tokens appear as inline badges at the left of the input. Click the chevron to add more filters. Backspace on an empty input removes the last token.`}}}},p={render:()=>{let[e,t]=(0,c.useState)(`John works at Acme Corp`);return(0,l.jsx)(s,{value:e,onChange:t,sourceLocale:`en-US`,targetLocale:`fr-FR`,onLookup:async()=>[{entry:{id:`1`,project_id:``,hint_src_lang:`en-US`,variants:{"en-US":{locale:`en-US`,text:`Bob works at Widget Inc`,coded:`Bob works at Widget Inc`,spans:[]},"fr-FR":{locale:`fr-FR`,text:`Bob travaille chez Widget Inc`,coded:`Bob travaille chez Widget Inc`,spans:[]}},created_at:new Date().toISOString(),updated_at:new Date().toISOString()},score:.85,match_type:`generalized-fuzzy`}]})},parameters:{docs:{description:{story:`Select text to mark entities, then press Enter to trigger lookup. Try selecting 'John' and marking as Person.`}}}},m={render:()=>{let[e,t]=(0,c.useState)(``);return(0,l.jsx)(s,{value:e,onChange:t,sourceLocale:`en-US`,targetLocale:`fr-FR`,placeholder:a(`1LGkTK`,`Type to search translations...`)})}},h={render:()=>{let[e,t]=(0,c.useState)(`Acme Corp hired John`),[n,r]=(0,c.useState)([]);return(0,l.jsxs)(`div`,{children:[(0,l.jsx)(s,{value:e,onChange:t,onEntitiesChange:r,onLookup:async()=>[],sourceLocale:`en-US`,targetLocale:`fr-FR`}),(0,l.jsxs)(`div`,{className:`mt-4 rounded-lg border bg-muted/30 p-3`,children:[(0,l.jsx)(`div`,{className:`text-[10px] font-semibold uppercase tracking-wider text-muted-foreground mb-1`,children:a(`2iDE5D`,`Search filter state (from onEntitiesChange)`)}),(0,l.jsx)(`pre`,{className:`text-[11px] text-foreground`,children:JSON.stringify({entity_values:n.map(e=>({value:e.text,type:e.type}))},null,2)})]})]})},parameters:{docs:{description:{story:`Select 'Acme Corp' and mark as Organization, then 'John' as Person. The Filter state panel updates live, showing how the parent component would build a TMSearchFilter to pass to the backend.`}}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [value, setValue] = useState("");
    return <TMSearchBar value={value} onChange={setValue} sourceLocale="en-US" targetLocale="fr-FR" />;
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [value, setValue] = useState("");
    const [filters, setFilters] = useState([{
      key: "language",
      value: "fr-FR"
    }, {
      key: "project",
      value: "webapp"
    }]);
    return <TMSearchBar value={value} onChange={setValue} filters={filters} onFiltersChange={setFilters} filterFields={[{
      key: "language",
      label: "Target Language",
      values: [{
        value: "fr-FR",
        label: "French (fr-FR)"
      }, {
        value: "de-DE",
        label: "German (de-DE)"
      }, {
        value: "ja-JP",
        label: "Japanese (ja-JP)"
      }]
    }, {
      key: "project",
      label: "Project",
      values: [{
        value: "webapp",
        label: "Web App"
      }, {
        value: "mobile",
        label: "Mobile"
      }]
    }]} sourceLocale="en-US" targetLocale="fr-FR" />;
  },
  parameters: {
    docs: {
      description: {
        story: "Filter tokens appear as inline badges at the left of the input. Click the chevron to add more filters. Backspace on an empty input removes the last token."
      }
    }
  }
}`,...f.parameters?.docs?.source},description:{story:`Search bar with filter tokens for language and project.`,...f.parameters?.docs?.description}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [value, setValue] = useState("John works at Acme Corp");
    return <TMSearchBar value={value} onChange={setValue} sourceLocale="en-US" targetLocale="fr-FR" onLookup={async () => [{
      entry: {
        id: "1",
        project_id: "",
        hint_src_lang: "en-US",
        variants: {
          "en-US": {
            locale: "en-US",
            text: "Bob works at Widget Inc",
            coded: "Bob works at Widget Inc",
            spans: []
          },
          "fr-FR": {
            locale: "fr-FR",
            text: "Bob travaille chez Widget Inc",
            coded: "Bob travaille chez Widget Inc",
            spans: []
          }
        },
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString()
      },
      score: 0.85,
      match_type: "generalized-fuzzy"
    }]} />;
  },
  parameters: {
    docs: {
      description: {
        story: "Select text to mark entities, then press Enter to trigger lookup. Try selecting 'John' and marking as Person."
      }
    }
  }
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [value, setValue] = useState("");
    return <TMSearchBar value={value} onChange={setValue} sourceLocale="en-US" targetLocale="fr-FR" placeholder="Type to search translations..." />;
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [value, setValue] = useState("Acme Corp hired John");
    const [entities, setEntities] = useState<Array<{
      text: string;
      type: string;
      start: number;
      end: number;
    }>>([]);
    return <div>
        <TMSearchBar value={value} onChange={setValue} onEntitiesChange={setEntities} onLookup={async () => []} sourceLocale="en-US" targetLocale="fr-FR" />
        <div className="mt-4 rounded-lg border bg-muted/30 p-3">
          <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground mb-1">
            Search filter state (from onEntitiesChange)
          </div>
          <pre className="text-[11px] text-foreground">
            {JSON.stringify({
            entity_values: entities.map(e => ({
              value: e.text,
              type: e.type
            }))
          }, null, 2)}
          </pre>
        </div>
      </div>;
  },
  parameters: {
    docs: {
      description: {
        story: "Select 'Acme Corp' and mark as Organization, then 'John' as Person. The Filter state panel updates live, showing how the parent component would build a TMSearchFilter to pass to the backend."
      }
    }
  }
}`,...h.parameters?.docs?.source},description:{story:`Demonstrates the entity-value filter flow. When the user marks text in
the search bar as an entity, the parent component receives the marked
entities via onEntitiesChange and converts them to search filters.
This gives precise entity-aware browser filtering — "find all entries
where Acme Corp is tagged as an Organization" — distinct from plain
text search.

Select text in the input and mark it with an entity type; the Filter
state panel below shows how the parent would build a search filter.`,...h.parameters?.docs?.description}}},g=[`Default`,`WithFilterTokens`,`WithLookup`,`CustomPlaceholder`,`EntityValueFilter`]}))();export{m as CustomPlaceholder,d as Default,h as EntityValueFilter,f as WithFilterTokens,p as WithLookup,g as __namedExportsOrder,u as default};