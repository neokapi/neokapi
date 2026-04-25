import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,r as a,s as o,t as s}from"./runtime-B587fM_0.js";import{M as c,Z as l,a as u,ot as d,t as f,w as p,z as m}from"./lucide-react-CNTu2eXS.js";import{t as h}from"./src-B5GKo4pn.js";import{t as g}from"./button-Dd5Xmast.js";import{o as _,r as v,s as y,t as b}from"./card-DEFJwrtm.js";import{t as x}from"./badge-DDhlaGcs.js";import{n as S,t as C}from"./useApi-C-HfJNVP.js";import{n as w,r as T,t as E}from"./api-D5xnl73H.js";function D(e,t){if(e.name)return e.name;if(!t)return`Untitled`;let n=t.replace(/\/project\.kapi$/i,``).split(`/`);return n[n.length-1]||`Untitled`}function O({project:e,projectPath:t,onSaved:n,onProjectChange:r,tabID:a}){let[f,h]=(0,k.useState)(!1),[S,w]=(0,k.useState)(!1),[O,j]=(0,k.useState)(``),M=e.defaults??{},N=e.plugins??{},P=async()=>{h(!0);try{if(t)await C.saveProject(a);else{let e=await C.saveProjectDialog(a);e&&n?.(e)}}finally{h(!1)}},F=(0,k.useCallback)(()=>{j(e.name||``),w(!0)},[e.name]),I=(0,k.useCallback)(async()=>{let n=O.trim(),i={...e,name:n};await C.updateProject(a,i),r?.(i),t&&await C.saveProject(a),w(!1)},[O,e,a,t,r]),L=(0,k.useCallback)(()=>{w(!1)},[]),R=D(e,t);return(0,A.jsxs)(`div`,{className:`p-6`,children:[(0,A.jsxs)(`div`,{className:`mb-6 flex items-center justify-between`,children:[(0,A.jsxs)(`div`,{children:[S?(0,A.jsxs)(`div`,{className:`flex items-center gap-2`,children:[(0,A.jsx)(`input`,{type:`text`,value:O,onChange:e=>j(e.target.value),onKeyDown:e=>{e.key===`Enter`&&I(),e.key===`Escape`&&L()},placeholder:D({...e,name:``},t),autoFocus:!0,className:`rounded-md border border-input bg-transparent px-2 py-1 text-xl font-semibold outline-none focus:ring-2 focus:ring-ring`}),(0,A.jsx)(g,{variant:`outline`,size:`xs`,onClick:I,children:s(`3z360r`,`Save`)}),(0,A.jsx)(g,{variant:`outline`,size:`xs`,onClick:L,children:s(`NwIbc`,`Cancel`)})]}):(0,A.jsxs)(`div`,{className:`group flex items-center gap-2`,children:[(0,A.jsx)(`h1`,{className:`text-xl font-semibold`,children:R}),(0,A.jsx)(g,{variant:`ghost`,size:`icon-xs`,onClick:F,className:`opacity-0 group-hover:opacity-100`,"aria-label":s(`2aDtWZ`,`Edit project name`),children:(0,A.jsx)(c,{size:14})})]}),t?(0,A.jsx)(`p`,{className:`mt-1 text-sm text-muted-foreground`,children:t}):(0,A.jsx)(`p`,{className:`mt-1 text-sm text-muted-foreground`,children:s(`19MQoW`,`Not yet saved to disk`)})]}),(0,A.jsxs)(g,{variant:`outline`,size:`sm`,onClick:P,disabled:f,"aria-label":t?s(`2DVk9G`,`Save project`):s(`3KSrCH`,`Save project as`),children:[f?(0,A.jsx)(m,{size:12,className:`animate-spin`}):(0,A.jsx)(p,{size:12}),t?s(`46EZ0n`,`Save`):s(`3VyNpV`,`Save As...`)]})]}),(0,A.jsxs)(`div`,{className:`grid grid-cols-1 gap-4 md:grid-cols-3`,children:[(0,A.jsxs)(b,{children:[(0,A.jsx)(_,{className:`px-4`,children:(0,A.jsx)(y,{className:`flex items-center gap-2 text-sm font-medium`,children:i(`38vzcP`,`{=m0} Languages`,{"=m0":(0,A.jsx)(l,{size:16,className:`text-primary`})})})}),(0,A.jsx)(v,{className:`px-4`,children:(0,A.jsxs)(`div`,{className:`space-y-1 text-sm`,children:[(0,A.jsx)(`div`,{children:i(`1S56jU`,`{=m0} {=m1}`,{"=m0":(0,A.jsx)(`span`,{className:`text-muted-foreground`,children:`Source: `}),"=m1":(0,A.jsx)(`span`,{children:M.source_language||`Not set`})})}),(0,A.jsx)(`div`,{children:i(`1S56jU`,`{=m0} {=m1}`,{"=m0":(0,A.jsx)(`span`,{className:`text-muted-foreground`,children:`Targets: `}),"=m1":(0,A.jsx)(`span`,{children:M.target_languages?.length?M.target_languages.join(`, `):o(`None`)})})})]})})]}),(0,A.jsxs)(b,{children:[(0,A.jsx)(_,{className:`px-4`,children:(0,A.jsx)(y,{className:`flex items-center gap-2 text-sm font-medium`,children:i(`36HhG0`,`{=m0} Content`,{"=m0":(0,A.jsx)(d,{size:16,className:`text-primary`})})})}),(0,A.jsx)(v,{className:`px-4`,children:(0,A.jsx)(`div`,{className:`space-y-1 text-sm`,children:e.content?.length?e.content.map((e,t)=>T(e)?(0,A.jsxs)(`div`,{className:`truncate text-muted-foreground`,children:[e.path,e.format&&(0,A.jsx)(`span`,{className:`ml-1 text-xs`,children:s(`1tToqL`,`(${e.format.name})`,{name:e.format.name})})]},t):(0,A.jsxs)(`div`,{className:`text-muted-foreground`,children:[e.name||s(`4wsYZq`,`Unnamed`),(0,A.jsx)(`span`,{className:`ml-1 text-xs`,children:s(`2yywgh`,`({count} item(s))`,{count:E(e).length})})]},t)):(0,A.jsx)(`p`,{className:`text-muted-foreground`,children:s(`2X3rte`,`No content patterns`)})})})]}),(0,A.jsxs)(b,{children:[(0,A.jsx)(_,{className:`px-4`,children:(0,A.jsx)(y,{className:`flex items-center gap-2 text-sm font-medium`,children:i(`2ficrV`,`{=m0} Flows`,{"=m0":(0,A.jsx)(u,{size:16,className:`text-primary`})})})}),(0,A.jsx)(v,{className:`px-4`,children:(0,A.jsx)(`div`,{className:`space-y-1 text-sm`,children:e.flows&&Object.keys(e.flows).length>0?Object.entries(e.flows).map(([e,t])=>(0,A.jsxs)(`div`,{className:`text-muted-foreground`,children:[e,(0,A.jsx)(`span`,{className:`ml-1 text-xs`,children:s(`3576Ht`,`({count} step(s))`,{count:t.steps.length})})]},e)):(0,A.jsx)(`p`,{className:`text-muted-foreground`,children:s(`4dgvBN`,`No flows defined`)})})})]})]}),(e.preset||Object.keys(N).length>0)&&(0,A.jsxs)(`div`,{className:`mt-6 space-y-2 text-sm`,children:[e.preset&&(0,A.jsx)(`div`,{children:i(`pKrtN`,`{=m0} {=m1}`,{"=m0":(0,A.jsx)(`span`,{className:`text-muted-foreground`,children:`Preset: `}),"=m1":(0,A.jsx)(x,{variant:`secondary`,children:e.preset})})}),Object.keys(N).length>0&&(0,A.jsx)(`div`,{children:i(`pKrtN`,`{=m0} {=m1}`,{"=m0":(0,A.jsx)(`span`,{className:`text-muted-foreground`,children:`Plugins: `}),"=m1":Object.entries(N).map(([e,t])=>(0,A.jsxs)(x,{variant:`secondary`,className:`mr-1`,translate:`no`,children:[e,t.version&&t.version!==`*`?` ${t.version}`:``]},e))})})]})]})}var k,A,j=t((()=>{a(),k=e(n(),1),f(),h(),w(),S(),A=r(),O.__docgenInfo={description:``,methods:[],displayName:`ProjectPage`,props:{project:{required:!0,tsType:{name:`KapiProject`},description:``},projectPath:{required:!0,tsType:{name:`string`},description:``},onSaved:{required:!1,tsType:{name:`signature`,type:`function`,raw:`(tab: TabInfo) => void`,signature:{arguments:[{type:{name:`TabInfo`},name:`tab`}],return:{name:`void`}}},description:``},onProjectChange:{required:!1,tsType:{name:`signature`,type:`function`,raw:`(project: KapiProject) => void`,signature:{arguments:[{type:{name:`KapiProject`},name:`project`}],return:{name:`void`}}},description:``},tabID:{required:!0,tsType:{name:`string`},description:``}}}})),M,N,P,F,I;t((()=>{j(),M={title:`Pages/ProjectPage`,component:O,tags:[`autodocs`],args:{tabID:`story-tab`}},N={args:{project:{version:`v1`,name:`Acme App Localization`,defaults:{source_language:`en-US`,target_languages:[`fr-FR`,`de-DE`,`ja-JP`]},content:[{path:`src/i18n/en/*.json`,format:{name:`json`},target:`src/i18n/{lang}/*.json`},{name:`Documentation`,items:[{path:`docs/en/**/*.md`,format:{name:`markdown`}}]}],preset:`nextjs`,plugins:{okapi:{framework_version:`^1.47.0`}},flows:{translate:{steps:[{tool:`ai-translate`,config:{provider:`anthropic`}}]},"translate-and-qa":{steps:[{tool:`ai-translate`,config:{provider:`anthropic`}},{tool:`qa-check`}]}}},projectPath:`/Users/dev/acme-app/translation.kapi`}},P={args:{project:{version:`v1`,name:`New Project`,defaults:{source_language:`en`}},projectPath:``}},F={args:{project:{version:`v1`,name:`QA Pipeline`,defaults:{source_language:`en-US`,target_languages:[`fr-FR`]},flows:{"qa-check":{steps:[{tool:`qa-check`}]},pseudo:{steps:[{tool:`pseudo-translate`,config:{expansion_rate:1.3}}]}}},projectPath:`/tmp/qa.kapi`}},N.parameters={...N.parameters,docs:{...N.parameters?.docs,source:{originalSource:`{
  args: {
    project: {
      version: "v1",
      name: "Acme App Localization",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR", "de-DE", "ja-JP"]
      },
      content: [{
        path: "src/i18n/en/*.json",
        format: {
          name: "json"
        },
        target: "src/i18n/{lang}/*.json"
      }, {
        name: "Documentation",
        items: [{
          path: "docs/en/**/*.md",
          format: {
            name: "markdown"
          }
        }]
      }],
      preset: "nextjs",
      plugins: {
        okapi: {
          framework_version: "^1.47.0"
        }
      },
      flows: {
        translate: {
          steps: [{
            tool: "ai-translate",
            config: {
              provider: "anthropic"
            }
          }]
        },
        "translate-and-qa": {
          steps: [{
            tool: "ai-translate",
            config: {
              provider: "anthropic"
            }
          }, {
            tool: "qa-check"
          }]
        }
      }
    },
    projectPath: "/Users/dev/acme-app/translation.kapi"
  }
}`,...N.parameters?.docs?.source}}},P.parameters={...P.parameters,docs:{...P.parameters?.docs,source:{originalSource:`{
  args: {
    project: {
      version: "v1",
      name: "New Project",
      defaults: {
        source_language: "en"
      }
    },
    projectPath: ""
  }
}`,...P.parameters?.docs?.source}}},F.parameters={...F.parameters,docs:{...F.parameters?.docs,source:{originalSource:`{
  args: {
    project: {
      version: "v1",
      name: "QA Pipeline",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR"]
      },
      flows: {
        "qa-check": {
          steps: [{
            tool: "qa-check"
          }]
        },
        pseudo: {
          steps: [{
            tool: "pseudo-translate",
            config: {
              expansion_rate: 1.3
            }
          }]
        }
      }
    },
    projectPath: "/tmp/qa.kapi"
  }
}`,...F.parameters?.docs?.source}}},I=[`WithContent`,`Minimal`,`WithFlowsOnly`]}))();export{P as Minimal,N as WithContent,F as WithFlowsOnly,I as __namedExportsOrder,M as default};