import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{n as o,t as s}from"./schema-form-ZLxpVH7H.js";function c({schema:e,initialValues:t={},compact:n=!1}){let[r,i]=(0,l.useState)(t),s=e.properties&&Object.keys(e.properties).length>0;return(0,u.jsxs)(`div`,{className:`grid grid-cols-[1fr_1fr] gap-6 max-w-[1100px]`,children:[(0,u.jsx)(`div`,{children:(0,u.jsx)(o,{schema:e,values:r,onChange:i,compact:n})}),(0,u.jsxs)(`div`,{className:`min-w-0`,children:[(0,u.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2`,children:a(`3TBNvC`,`Schema`)}),(0,u.jsx)(`pre`,{className:`rounded-md bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-[600px]`,children:JSON.stringify(e,null,2)}),s&&(0,u.jsxs)(u.Fragment,{children:[(0,u.jsx)(`h4`,{className:`text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2`,children:a(`1DEUwR`,`Values`)}),(0,u.jsx)(`pre`,{className:`rounded-md bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-[200px]`,children:JSON.stringify(r,null,2)})]})]})]})}var l,u,d,f,p,m,h,g,_,v,y,b,x,S,C,w,T,E,D;t((()=>{i(),l=e(n(),1),s(),u=r(),d={title:`Pseudo Translate`,type:`object`,toolMeta:{id:`pseudo-translate`,category:`transform`},properties:{prefix:{type:`string`,title:`Prefix`,default:`[`,description:`Prefix added to translations`},suffix:{type:`string`,title:`Suffix`,default:`]`,description:`Suffix added to translations`},expansionPercent:{type:`integer`,title:`Expansion Percent`,default:30,minimum:0,maximum:200,description:`Expand text length by percentage`},applyAccents:{type:`boolean`,title:`Apply Accents`,default:!0,description:`Apply diacritical marks to simulate translated text`}}},f={title:`Quality Check`,type:`object`,toolMeta:{id:`qa-check`,category:`validate`},"ui:groups":[{id:`whitespace`,label:`Whitespace Checks`,fields:[`checkLeadingWhitespace`,`checkTrailingWhitespace`,`checkDoubleSpaces`]},{id:`content`,label:`Content Checks`,fields:[`checkEmptyTarget`,`targetSameAsSource`]},{id:`output`,label:`Output`,fields:[`reportPath`,`reportFormat`]}],properties:{checkLeadingWhitespace:{type:`boolean`,title:`Check Leading Whitespace`,default:!0,description:`Check for leading whitespace differences`},checkTrailingWhitespace:{type:`boolean`,title:`Check Trailing Whitespace`,default:!0,description:`Check for trailing whitespace differences`},checkDoubleSpaces:{type:`boolean`,title:`Check Double Spaces`,default:!1,description:`Check for double spaces`},checkEmptyTarget:{type:`boolean`,title:`Warn on Empty Target`,default:!0,description:`Warn when target is empty`},targetSameAsSource:{type:`boolean`,title:`Target Same as Source`,default:!0,description:`Warn when target equals source`},reportPath:{type:`string`,title:`Report File Path`,default:"${rootDir}/qa-report.html","ui:widget":`path`,description:`Output report file path`},reportFormat:{type:`string`,title:`Report Format`,options:[{value:`html`,label:`HTML file`},{value:`tsv`,label:`Tab-delimited file`},{value:`xml`,label:`XML file`}],default:`html`,description:`Report output format`}}},p={title:`Batch Translation`,type:`object`,properties:{useTM:{type:`boolean`,title:`Use Translation Memory`,default:!1,description:`Use translation memory for leveraging`},tmPath:{type:`string`,title:`TM File Path`,description:`Path to translation memory file`,"ui:widget":`path`,"ui:enabled":{field:`useTM`,eq:!0}},threshold:{type:`integer`,title:`Match Threshold`,default:95,minimum:0,maximum:100,description:`Minimum match threshold`,"ui:enabled":{field:`useTM`,eq:!0}},markAsMT:{type:`boolean`,title:`Mark as Machine Translated`,default:!0,description:`Mark leveraged segments as machine translated`,"ui:visible":{field:`useTM`,eq:!0}}}},m={title:`Widget Showcase`,type:`object`,properties:{textField:{type:`string`,title:`Text Input`,description:`Simple text field`},password:{type:`string`,title:`Password`,"ui:widget":`password`},textarea:{type:`string`,title:`Code`,"ui:widget":`textarea`},regex:{type:`string`,title:`Pattern`,"ui:widget":`regex`},tags:{type:`string`,title:`Tags`,"ui:widget":`tags`},toggle:{type:`boolean`,title:`Enable feature`,default:!1},count:{type:`integer`,title:`Count`,minimum:0,maximum:100,default:10},mode:{type:`string`,title:`Mode`,options:[{value:`fast`,label:`Fast`},{value:`balanced`,label:`Balanced`},{value:`thorough`,label:`Thorough`}],default:`balanced`},segmented:{type:`string`,title:`Output Type`,"ui:widget":`segmented`,enum:[`source`,`target`,`both`]},codeFinder:{type:`object`,title:`Inline Codes`,"ui:widget":`code-finder`,"ui:presets":{"HTML Tags":{rules:[{pattern:`</?\\w[^>]*>`}],sample:`<b>Bold</b>`},Printf:{rules:[{pattern:`%[ds]`}],sample:`Found %d items`}}}}},h={title:`Array & List Editors`,type:`object`,properties:{tags:{type:`array`,title:`Simple Tags`,description:`String array rendered as pill chips`,items:{type:`string`}},patterns:{type:`array`,title:`Extraction Patterns`,description:`Structured array with per-item fields`,items:{type:`object`,properties:{pattern:{type:`string`,title:`Regex Pattern`},enabled:{type:`boolean`,title:`Enabled`,default:!0}}}},extensions:{type:`array`,title:`File Extensions`,description:`Simple string list`,items:{type:`string`}}}},g={title:`Map & Object Editors`,type:`object`,properties:{variables:{type:`object`,title:`Environment Variables`,description:`Key-value pairs — simple string values`,additionalProperties:{type:`string`}},elementRules:{type:`object`,title:`Element Rules`,description:`Complex map — each entry has structured properties`,"ui:widget":`element-rules`,additionalProperties:{type:`object`,properties:{ruleType:{type:`string`,title:`Rule Type`,enum:[`INLINE`,`TEXTUNIT`,`EXCLUDE`]},translatable:{type:`boolean`,title:`Translatable`,default:!0}}}},settings:{type:`object`,title:`Raw JSON Object`,description:`Untyped object — rendered as JSON editor`}}},_={title:`Nested Object Editor`,type:`object`,properties:{parser:{type:`object`,title:`Parser Settings`,description:`Nested object rendered inline at depth 0`,properties:{encoding:{type:`string`,title:`Encoding`,default:`UTF-8`},strict:{type:`boolean`,title:`Strict Mode`,default:!1},whitespace:{type:`object`,title:`Whitespace Handling`,description:`Deeper nesting — rendered as drill-down at depth 2+`,properties:{preserve:{type:`boolean`,title:`Preserve Whitespace`,default:!1},normalize:{type:`boolean`,title:`Normalize Spaces`,default:!0},trimLines:{type:`boolean`,title:`Trim Lines`,default:!1}}}}}}},v={title:`Formats & Tools/Schema/SchemaForm`,component:o,tags:[`autodocs`],parameters:{docs:{description:{component:`Schema-driven configuration form. Auto-generates form fields from a ComponentSchema, supporting groups, conditional visibility/enablement, preset comparison, and 15+ widget types.`}}}},y={render:()=>(0,u.jsx)(c,{schema:d})},b={render:()=>(0,u.jsx)(c,{schema:f})},x={render:()=>(0,u.jsx)(c,{schema:p})},S={render:()=>(0,u.jsx)(c,{schema:m})},C={render:()=>(0,u.jsx)(c,{schema:d,compact:!0})},w={name:`Array & List Editors`,render:()=>(0,u.jsx)(c,{schema:h,initialValues:{tags:[`localization`,`i18n`,`okapi`],patterns:[{pattern:`</?\\w[^>]*>`,enabled:!0},{pattern:`\\{\\d+\\}`,enabled:!1}],extensions:[`.html`,`.htm`]}})},T={name:`Map & Object Editors`,render:()=>(0,u.jsx)(c,{schema:g,initialValues:{variables:{ROOT_DIR:`/projects/demo`,OUTPUT_DIR:`/output`,LANG:`fr-FR`},elementRules:{div:{ruleType:`TEXTUNIT`,translatable:!0},span:{ruleType:`INLINE`,translatable:!0},script:{ruleType:`EXCLUDE`,translatable:!1}},settings:{debug:!0,version:2}}})},E={name:`Nested Object Editor`,render:()=>(0,u.jsx)(c,{schema:_,initialValues:{parser:{encoding:`UTF-8`,strict:!1,whitespace:{preserve:!1,normalize:!0,trimLines:!1}}}})},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper schema={simpleSchema} />
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper schema={groupedSchema} />
}`,...b.parameters?.docs?.source}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper schema={conditionalSchema} />
}`,...x.parameters?.docs?.source}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper schema={widgetsSchema} />
}`,...S.parameters?.docs?.source}}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  render: () => <Wrapper schema={simpleSchema} compact />
}`,...C.parameters?.docs?.source}}},w.parameters={...w.parameters,docs:{...w.parameters?.docs,source:{originalSource:`{
  name: "Array & List Editors",
  render: () => <Wrapper schema={arraySchema} initialValues={{
    tags: ["localization", "i18n", "okapi"],
    patterns: [{
      pattern: "</?\\\\w[^>]*>",
      enabled: true
    }, {
      pattern: "\\\\{\\\\d+\\\\}",
      enabled: false
    }],
    extensions: [".html", ".htm"]
  }} />
}`,...w.parameters?.docs?.source}}},T.parameters={...T.parameters,docs:{...T.parameters?.docs,source:{originalSource:`{
  name: "Map & Object Editors",
  render: () => <Wrapper schema={mapSchema} initialValues={{
    variables: {
      ROOT_DIR: "/projects/demo",
      OUTPUT_DIR: "/output",
      LANG: "fr-FR"
    },
    elementRules: {
      div: {
        ruleType: "TEXTUNIT",
        translatable: true
      },
      span: {
        ruleType: "INLINE",
        translatable: true
      },
      script: {
        ruleType: "EXCLUDE",
        translatable: false
      }
    },
    settings: {
      debug: true,
      version: 2
    }
  }} />
}`,...T.parameters?.docs?.source}}},E.parameters={...E.parameters,docs:{...E.parameters?.docs,source:{originalSource:`{
  name: "Nested Object Editor",
  render: () => <Wrapper schema={nestedSchema} initialValues={{
    parser: {
      encoding: "UTF-8",
      strict: false,
      whitespace: {
        preserve: false,
        normalize: true,
        trimLines: false
      }
    }
  }} />
}`,...E.parameters?.docs?.source}}},D=[`Simple`,`WithGroups`,`ConditionalVisibility`,`AllWidgets`,`Compact`,`ArrayEditors`,`MapEditors`,`NestedObjects`]}))();export{S as AllWidgets,w as ArrayEditors,C as Compact,x as ConditionalVisibility,T as MapEditors,E as NestedObjects,y as Simple,b as WithGroups,D as __namedExportsOrder,v as default};