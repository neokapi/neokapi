import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,r as a,s as o,t as s}from"./runtime-B587fM_0.js";import{A as c,S as l,Z as u,a as d,i as f,j as p,l as m,ot as h,t as g,z as _}from"./lucide-react-CNTu2eXS.js";import{a as v,t as y}from"./src-CFqQ2E1s.js";import{t as b}from"./button-Dd5Xmast.js";import{t as x}from"./card-DEFJwrtm.js";import{t as S}from"./badge-DDhlaGcs.js";import{t as C}from"./action-card-BjEy4xdK.js";import{n as w,t as T}from"./useApi-C-HfJNVP.js";import{n as E,r as D,t as O}from"./api-D5xnl73H.js";import{n as k,t as A}from"./JobFeedContext-5RedV9JH.js";function j({project:e,displayName:t,tabID:n,onRunFlow:r,onNavigate:a,pluginsResolved:g,pluginIssues:y}){let{hasActive:w,activeJob:E}=k(),[A,j]=(0,M.useState)({});(0,M.useEffect)(()=>{n&&T.listFlows(n).then(e=>{if(!e)return;let t={};for(let n of e)t[n.name]=n;j(t)})},[n,e.flows]);let P=e.defaults??{},F=e.plugins??{},I=Object.keys(e.flows??{}),L=(e.content?.length??0)>0,R=e.content?.length??0,z=e.content?.reduce((e,t)=>e+(D(t)?1:O(t).length),0)??0,B=t=>{let n=e.flows?.[t];!n||!r||r(t,n)};return(0,N.jsxs)(`div`,{className:`p-6`,children:[(0,N.jsxs)(`div`,{className:`mb-6`,children:[(0,N.jsx)(`h1`,{className:`text-xl font-semibold`,children:t}),(0,N.jsx)(`div`,{className:`mt-2 flex flex-wrap items-center gap-3 text-sm text-muted-foreground`,children:i(`2cXlH7`,`{=m0} {=m1} {=m2}`,{"=m0":(0,N.jsxs)(`span`,{className:`flex items-center gap-1.5`,children:[(0,N.jsx)(u,{size:14}),P.source_language||o(`No source`),` →`,` `,P.target_languages?.length?P.target_languages.join(`, `):o(`No targets`)]}),"=m1":e.preset&&(0,N.jsx)(S,{variant:`secondary`,className:`text-xs`,children:e.preset}),"=m2":Object.keys(F).length>0&&Object.entries(F).map(([e,t])=>(0,N.jsxs)(`span`,{className:`flex items-center gap-1`,children:[(0,N.jsx)(c,{size:10}),(0,N.jsxs)(`span`,{className:`text-xs`,children:[e,t.framework_version&&(0,N.jsxs)(`span`,{className:`text-muted-foreground/60`,children:[` `,t.framework_version]})]})]},e))})})]}),g===!1&&y&&y.length>0&&(0,N.jsx)(`div`,{className:`mb-6 rounded-lg border border-amber-500/30 bg-amber-500/5 p-4`,children:(0,N.jsxs)(`div`,{className:`flex items-start gap-3`,children:[(0,N.jsx)(m,{size:16,className:`mt-0.5 shrink-0 text-amber-500`}),(0,N.jsxs)(`div`,{className:`flex-1`,children:[(0,N.jsx)(`p`,{className:`text-sm font-medium`,children:s(`3K5OJB`,`Plugin requirements not met`)}),(0,N.jsx)(`p`,{className:`mt-1 text-xs text-muted-foreground`,children:s(`4lNilC`,`This project requires plugins that are not installed or have incompatible versions. Content and flow features are disabled until this is resolved.`)}),(0,N.jsx)(`ul`,{className:`mt-2 space-y-1`,children:y.map(e=>(0,N.jsxs)(`li`,{className:`flex items-center gap-2 text-xs`,children:[(0,N.jsx)(S,{variant:`outline`,className:`text-[10px]`,children:e.plugin}),e.type===`missing`?(0,N.jsx)(`span`,{className:`text-muted-foreground`,children:s(`1Dyx95`,`not installed`)}):(0,N.jsx)(`span`,{className:`text-muted-foreground`,children:s(`3t7HJg`,`requires ${e.required}, installed ${e.installed_version}`,{"issue.required":e.required,"issue.installed_version":e.installed_version})})]},e.plugin))}),(0,N.jsxs)(`div`,{className:`mt-3 flex gap-2`,children:[(0,N.jsx)(b,{size:`sm`,variant:`outline`,onClick:()=>a(`project-settings`),children:i(`4dlxNt`,`{=m0} Edit Plugin Settings`,{"=m0":(0,N.jsx)(l,{size:12})})}),(0,N.jsx)(b,{size:`sm`,variant:`outline`,onClick:()=>a(`app-settings`),children:i(`6D1OJ`,`{=m0} Install Plugins`,{"=m0":(0,N.jsx)(c,{size:12})})})]})]})]})}),(0,N.jsxs)(`div`,{className:`mb-8 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4`,children:[(0,N.jsx)(C,{icon:(0,N.jsx)(h,{size:16}),title:s(`1TtjjF`,`Content`),description:L?s(`G5HFe`,`{contentCount} collection(s), {itemCount} pattern(s)`,{contentCount:R,itemCount:z}):s(`1d1kXs`,`Configure file patterns`),onClick:()=>a(`content`)}),(0,N.jsx)(C,{icon:(0,N.jsx)(d,{size:16}),title:s(`25KjwG`,`Flows`),description:I.length>0?s(`4v3lIL`,`{count} flow(s) defined`,{count:I.length}):s(`3ahfLk`,`Build your first flow`),onClick:()=>a(`flows`)}),(0,N.jsx)(C,{icon:(0,N.jsx)(f,{size:16}),title:s(`3Wwo9o`,`Tools`),description:s(`l47pm`,`Run individual tools on files`),onClick:()=>a(`tools`)}),(0,N.jsx)(C,{icon:(0,N.jsx)(l,{size:16}),title:s(`HIEtF`,`Settings`),description:s(`3y4FjI`,`Languages, plugins, processing`),onClick:()=>a(`project-settings`)})]}),I.length>0&&(0,N.jsxs)(`section`,{children:[(0,N.jsx)(`h2`,{className:`mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground`,children:i(`3YFIay`,`{=m0} Run Flows`,{"=m0":(0,N.jsx)(d,{size:14})})}),(0,N.jsx)(`div`,{className:`space-y-2`,children:I.map(t=>{let n=e.flows?.[t];if(!n)return null;let r=A[t],a=r?.valid!==!1,o=r?.issues??[],c=a&&L&&!w,l=a?L?void 0:`Configure content patterns first`:`Cannot run: ${o.map(e=>e.message).join(`; `)}`;return(0,N.jsxs)(x,{className:`flex flex-row items-center gap-3 p-3 ${a?``:`border-amber-500/30 bg-amber-500/5`}`,children:[(0,N.jsxs)(`div`,{className:`flex-1`,children:[(0,N.jsxs)(`div`,{className:`flex items-center gap-1.5`,children:[(0,N.jsx)(`span`,{className:`text-sm font-medium`,children:t}),!a&&(0,N.jsx)(m,{size:12,className:`text-amber-500`})]}),(0,N.jsx)(`div`,{className:`mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground`,children:n.steps.map((e,t)=>{let n=o.some(t=>t.tool===e.tool);return(0,N.jsxs)(`span`,{className:`flex items-center gap-1`,children:[t>0&&(0,N.jsx)(`span`,{children:s(`3C1wnn`,`→`)}),(0,N.jsx)(S,{variant:n?`destructive`:`secondary`,className:n?`line-through opacity-70`:``,children:e.tool})]},t)})}),o.length>0&&(0,N.jsx)(`div`,{className:`mt-1 text-[10px] text-amber-600 dark:text-amber-400`,children:o.map((e,t)=>(0,N.jsx)(`div`,{children:e.message},t))})]}),(0,N.jsx)(b,{size:`sm`,onClick:()=>B(t),disabled:!c,"aria-label":s(`2FvK9S`,`Run flow {name}`,{name:t}),title:l,children:i(`14DbVS`,`{=m0} Run`,{"=m0":E?.flowName===t?(0,N.jsx)(_,{size:12,className:`animate-spin`}):(0,N.jsx)(p,{size:12})})})]},t)})})]}),I.length===0&&(0,N.jsx)(v,{icon:(0,N.jsx)(d,{size:24,className:`text-muted-foreground/50`}),title:s(`1CrUtt`,`No flows defined yet.`),action:(0,N.jsx)(b,{variant:`link`,size:`sm`,onClick:()=>a(`flows`),className:`h-auto px-0`,children:s(`uOItw`,`Create your first flow`)})})]})}var M,N,P=t((()=>{a(),M=e(n(),1),g(),y(),E(),w(),A(),N=r(),j.__docgenInfo={description:``,methods:[],displayName:`HomePage`,props:{project:{required:!0,tsType:{name:`KapiProject`},description:``},displayName:{required:!0,tsType:{name:`string`},description:``},tabID:{required:!1,tsType:{name:`string`},description:``},onRunFlow:{required:!1,tsType:{name:`signature`,type:`function`,raw:`(flowName: string, flow: FlowSpec) => void`,signature:{arguments:[{type:{name:`string`},name:`flowName`},{type:{name:`FlowSpec`},name:`flow`}],return:{name:`void`}}},description:``},onNavigate:{required:!0,tsType:{name:`signature`,type:`function`,raw:`(view: string) => void`,signature:{arguments:[{type:{name:`string`},name:`view`}],return:{name:`void`}}},description:``},pluginsResolved:{required:!1,tsType:{name:`boolean`},description:`When false, plugin requirements are unmet — show warning banner.`},pluginIssues:{required:!1,tsType:{name:`Array`,elements:[{name:`PluginIssue`}],raw:`PluginIssue[]`},description:`Details of unsatisfied plugin requirements.`}}}})),F,I,L,R,z;t((()=>{P(),{fn:F}=__STORYBOOK_MODULE_TEST__,I={title:`Pages/HomePage`,component:j,tags:[`autodocs`],args:{onRunFlow:F(),onNavigate:F()}},L={args:{displayName:`Acme App Localization`,project:{version:`v1`,name:`Acme App Localization`,defaults:{source_language:`en-US`,target_languages:[`fr-FR`,`de-DE`,`ja-JP`]},plugins:{okapi:{framework_version:`^1.47.0`,format_priority:200}},preset:`nextjs`,content:[{path:`src/i18n/en/*.json`,format:{name:`json`},target:`src/i18n/{lang}/*.json`},{path:`docs/en/**/*.md`,format:{name:`markdown`}}],flows:{translate:{steps:[{tool:`ai-translate`,config:{provider:`anthropic`}}]},"translate-and-qa":{steps:[{tool:`ai-translate`,config:{provider:`anthropic`}},{tool:`qa-check`}]}}}}},R={args:{displayName:`Starter Project`,project:{version:`v1`,name:`Starter Project`,defaults:{source_language:`en-US`,target_languages:[`fr-FR`]},content:[{path:`src/locales/en.json`,format:{name:`json`}}]}}},L.parameters={...L.parameters,docs:{...L.parameters?.docs,source:{originalSource:`{
  args: {
    displayName: "Acme App Localization",
    project: {
      version: "v1",
      name: "Acme App Localization",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR", "de-DE", "ja-JP"]
      },
      plugins: {
        okapi: {
          framework_version: "^1.47.0",
          format_priority: 200
        }
      },
      preset: "nextjs",
      content: [{
        path: "src/i18n/en/*.json",
        format: {
          name: "json"
        },
        target: "src/i18n/{lang}/*.json"
      }, {
        path: "docs/en/**/*.md",
        format: {
          name: "markdown"
        }
      }],
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
    }
  }
}`,...L.parameters?.docs?.source}}},R.parameters={...R.parameters,docs:{...R.parameters?.docs,source:{originalSource:`{
  args: {
    displayName: "Starter Project",
    project: {
      version: "v1",
      name: "Starter Project",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR"]
      },
      content: [{
        path: "src/locales/en.json",
        format: {
          name: "json"
        }
      }]
    }
  }
}`,...R.parameters?.docs?.source}}},z=[`Default`,`NoFlows`]}))();export{L as Default,R as NoFlows,z as __namedExportsOrder,I as default};