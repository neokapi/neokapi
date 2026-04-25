import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,r as a,t as o}from"./runtime-B587fM_0.js";import{D as s,Et as ee,M as c,Ot as te,S as ne,U as re,Z as ie,c as ae,dt as oe,ft as se,k as l,ot as u,t as d,z as ce}from"./lucide-react-CNTu2eXS.js";import{_ as le,g as f,h as ue,m as de,t as p,v as fe}from"./src-CFqQ2E1s.js";import{t as m}from"./label-Cm4mc8Sq.js";import{t as h}from"./button-Dd5Xmast.js";import{t as g}from"./card-DEFJwrtm.js";import{t as _}from"./badge-DDhlaGcs.js";import{r as pe,t as me}from"./target-path-input-Dk07-U1j.js";import{t as v}from"./item-card-DGYr6ylt.js";import{t as y}from"./confirm-delete-button-C4rBI4_z.js";import{t as he}from"./format-select-D7aZbjqc.js";import{n as ge,t as _e}from"./locale-select-BnmiVczV.js";import{n as b,r as ve}from"./iframe-BY2JU_X5.js";import{n as x,t as S}from"./useApi-C-HfJNVP.js";import{n as ye,t as C}from"./useShortenHome-Bp5A9YM8.js";import{n as w,r as be,t as xe}from"./api-D5xnl73H.js";import{n as T,t as Se}from"./TranslationStatusPanel-BqZ_Spwi.js";import{n as Ce,t as E}from"./useWailsEvent-BWwtX5bs.js";import{n as we,t as D}from"./useLocales-ByoNOItU.js";function Te(e){return e<1024?`${e} B`:e<1024*1024?`${(e/1024).toFixed(1)} KB`:`${(e/(1024*1024)).toFixed(1)} MB`}function O(e){return e?.name??``}function k({project:e,projectPath:t,onUpdate:n,tabID:r,formatList:a,basePath:d}){let{showError:p}=ve(),{locales:b}=we(),x=ye(),[C,w]=(0,A.useState)([]),[T,E]=(0,A.useState)([]),[D,k]=(0,A.useState)(d??``),[M,N]=(0,A.useState)(!1),[P,F]=(0,A.useState)(a??[]),[I,L]=(0,A.useState)(!1),[R,z]=(0,A.useState)(!1),[B,V]=(0,A.useState)({}),[Ee,De]=(0,A.useState)(new Set),[Oe,ke]=(0,A.useState)(new Set),H=e.content??[],U=(0,A.useMemo)(()=>{let e=new Set;for(let t of H)for(let n of xe(t)){let t=O(n.format);t&&e.add(t)}return[...e]},[H]),W=!!(a&&d);(0,A.useEffect)(()=>{if(!W)for(let e of U)B[e]||S.listFormatPresets(e).then(t=>{t&&V(n=>({...n,[e]:t}))})},[U,W]),(0,A.useEffect)(()=>{a||S.listFormats().then(e=>{e&&F(e)}).catch(e=>p(`Failed to load formats`,e)),d||S.getBasePath(r).then(e=>{e&&k(e)}).catch(e=>p(`Failed to get base path`,e))},[r,p,a,d]);let G=(0,A.useCallback)(async()=>{if(!W){N(!0);try{await S.updateProject(r,e);let[t,n]=await Promise.all([S.matchContent(r),S.listProjectFiles(r)]);w(t??[]),E(n??[])}catch(e){p(`Failed to scan files`,e)}finally{N(!1)}}},[r,e,p,W]);(0,A.useEffect)(()=>{G()},[G,H.length]),Ce(`project-files-changed`,e=>{e===r&&G()});let K=t=>{n({...e,content:t})},Ae=()=>{K([...H,{name:`New Collection`,items:[{path:``}]}])},q=(e,t)=>{let n=[...H];n[e]=t,K(n)},J=e=>{K(H.filter((t,n)=>n!==e))},je=async()=>{let e=await S.addFilesDialog(r,``);e&&e.length>0&&G()},Y=(0,A.useCallback)(async e=>{e.preventDefault(),z(!1);let t=e.dataTransfer?.files;if(!(!t||t.length===0)){for(let e=0;e<t.length;e++){let n=t[e].path;n&&await S.copyFileToProject(r,n,``)}G()}},[r,G]),Me=(0,A.useCallback)(e=>{e.preventDefault(),z(!0)},[]),Ne=(0,A.useCallback)(e=>{e.preventDefault(),z(!1)},[]),Pe=new Set(C.map(e=>e.relative)),X=new Map;for(let e of C){let t=e.collection||``,n=X.get(t)??[];n.push(e),X.set(t,n)}let Z=T.filter(e=>!e.is_dir&&!Pe.has(e.relative)),Fe=[...X.keys()].sort((e,t)=>e?t?e.localeCompare(t):-1:1),Q=T.filter(e=>!e.is_dir).length,$=(e,t,n)=>{let r=O(e.format),a=r?B[r]??[]:[],s=e.format?.config&&Object.keys(e.format.config).length>0,c=Ee.has(n);return(0,j.jsxs)(`div`,{className:`space-y-2`,children:[(0,j.jsxs)(`div`,{children:[(0,j.jsx)(m,{className:`mb-0.5 block text-xs text-muted-foreground`,children:o(`yBWxw`,`Path pattern`)}),(0,j.jsx)(pe,{value:e.path,onChange:n=>t({...e,path:n}),placeholder:o(`1R8M9g`,`src/locales/en/*.json`)})]}),(0,j.jsxs)(`div`,{className:`grid grid-cols-2 gap-2`,children:[(0,j.jsxs)(`div`,{children:[(0,j.jsx)(m,{className:`mb-0.5 block text-xs text-muted-foreground`,children:o(`39Lmr`,`Format`)}),(0,j.jsx)(he,{value:r,onChange:n=>{t({...e,format:n?{name:n}:void 0}),n&&!B[n]&&S.listFormatPresets(n).then(e=>{e&&V(t=>({...t,[n]:e}))})},formats:P})]}),(0,j.jsxs)(`div`,{children:[(0,j.jsx)(m,{className:`mb-0.5 block text-xs text-muted-foreground`,children:o(`3YGJcS`,`Target path`)}),(0,j.jsx)(me,{value:e.target??``,onChange:n=>t({...e,target:n||void 0}),placeholder:o(`4oh7KX`,`src/locales/{lang}/*.json`)})]})]}),r===`exec`&&(0,j.jsxs)(`div`,{children:[(0,j.jsx)(m,{className:`mb-0.5 block text-xs text-muted-foreground`,children:o(`1gbrZJ`,`Extractor command`)}),(0,j.jsx)(`input`,{type:`text`,value:typeof e.format?.config?.command==`string`?e.format.config.command:``,onChange:n=>t({...e,format:{...e.format,config:{...e.format?.config,command:n.target.value||void 0}}}),placeholder:o(`1YiMcB`,`vp kapi-react extract --stream`),className:`w-full rounded-md border border-input bg-background px-2 py-1 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring`}),(0,j.jsx)(`p`,{className:`mt-0.5 text-xs text-muted-foreground`,children:o(`4oM908`,"`kapi extract -p` runs this command; NUL-separated paths on stdin, NDJSON blocks on stdout.")})]}),r&&r!==`exec`&&(0,j.jsxs)(`div`,{children:[(0,j.jsx)(m,{className:`mb-0.5 block text-xs text-muted-foreground`,children:o(`4xOGP`,`Format Preset`)}),(0,j.jsxs)(de,{value:e.format?.preset||`__default__`,onValueChange:n=>t({...e,format:{...e.format,preset:n===`__default__`?void 0:n}}),children:[(0,j.jsx)(le,{className:`h-8 text-xs`,children:(0,j.jsx)(fe,{})}),(0,j.jsxs)(ue,{children:[(0,j.jsx)(f,{value:`__default__`,children:o(`25pjjn`,`Default`)}),a.map(e=>(0,j.jsxs)(f,{value:e.name,translate:`no`,children:[e.name,e.description?` \u2014 ${e.description}`:``]},e.name))]})]})]}),r&&(0,j.jsxs)(`div`,{children:[(0,j.jsx)(h,{variant:`ghost`,size:`xs`,onClick:()=>{De(e=>{let t=new Set(e);return t.has(n)?t.delete(n):t.add(n),t})},className:`h-auto px-0 text-muted-foreground hover:text-foreground`,children:i(`4vml2r`,`{=m0} {=m1} Format Config {=m2}`,{"=m0":c?(0,j.jsx)(ee,{size:10}):(0,j.jsx)(te,{size:10}),"=m1":(0,j.jsx)(ne,{size:10}),"=m2":s&&(0,j.jsx)(`span`,{className:`ml-1 rounded bg-primary/10 px-1.5 py-0.5 text-primary`,children:Object.keys(e.format.config).length})})}),c&&(0,j.jsxs)(`div`,{className:`mt-1.5`,children:[(0,j.jsx)(`textarea`,{value:s?JSON.stringify(e.format.config,null,2):``,onChange:n=>{let r=n.target.value.trim();if(!r){t({...e,format:{...e.format,config:void 0}});return}try{let n=JSON.parse(r);t({...e,format:{...e.format,config:n}})}catch{}},placeholder:o(`3pbdfK`,`{"key": "value"}`),rows:4,className:`w-full rounded border border-input bg-transparent px-2 py-1 font-mono text-xs outline-none focus:ring-1 focus:ring-ring`}),(0,j.jsx)(`p`,{className:`mt-0.5 text-xs text-muted-foreground`,children:o(`2wUfzo`,`JSON config passed to the format reader/writer.`)})]})]})]})};return(0,j.jsxs)(`div`,{className:`flex h-full flex-col overflow-hidden p-6`,children:[(0,j.jsxs)(`div`,{className:`mb-4 flex items-baseline justify-between`,children:[(0,j.jsx)(`h1`,{className:`text-xl font-semibold`,children:o(`3cDD56`,`Content`)}),D&&(0,j.jsx)(`p`,{className:`text-xs text-muted-foreground`,children:o(`4xDJXy`,`All paths relative to ${x(D)}`,{shortenHome:x(D)})})]}),H.some(e=>e.archive)&&(0,j.jsxs)(`section`,{className:`mb-4`,children:[(0,j.jsx)(`h2`,{className:`mb-2 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground`,children:o(`2AuCzA`,`Translation state`)}),(0,j.jsx)(Se,{tabID:r})]}),(0,j.jsxs)(`div`,{className:`grid min-h-0 flex-1 grid-cols-2 gap-6`,children:[(0,j.jsxs)(`section`,{className:`flex min-h-0 flex-col overflow-auto`,children:[(0,j.jsxs)(`div`,{className:`mb-3 flex items-center justify-between`,children:[(0,j.jsx)(`h2`,{className:`flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground`,children:i(`4hFy0H`,`{=m0} File Patterns`,{"=m0":(0,j.jsx)(u,{size:14})})}),(0,j.jsx)(h,{variant:`outline`,size:`sm`,onClick:Ae,"aria-label":o(`2oCmwt`,`Add content collection`),children:i(`jWLGo`,`{=m0} Add Collection`,{"=m0":(0,j.jsx)(l,{size:12})})})]}),H.length>0?(0,j.jsx)(`div`,{className:`space-y-3`,children:H.map((t,n)=>{if(be(t)){let e={path:t.path??``,format:t.format,target:t.target};return(0,j.jsxs)(v,{className:`relative`,children:[(0,j.jsx)(`div`,{className:`absolute right-2 top-2 opacity-0 group-hover:opacity-100`,children:(0,j.jsx)(y,{onDelete:()=>J(n),mode:`icon`})}),$(e,e=>q(n,{path:e.path,format:e.format,target:e.target}),`bare-${n}`)]},n)}return(0,j.jsxs)(v,{className:`p-0 overflow-hidden`,children:[(0,j.jsxs)(`div`,{className:`flex items-center justify-between border-b border-border px-4 py-3`,children:[(0,j.jsxs)(`div`,{className:`flex items-center gap-2`,children:[(0,j.jsx)(re,{size:14,className:`text-primary`}),(0,j.jsx)(`input`,{type:`text`,value:t.name??``,onChange:e=>q(n,{...t,name:e.target.value||void 0}),placeholder:o(`XOfXj`,`Collection name`),className:`bg-transparent text-sm font-medium outline-none placeholder:text-muted-foreground/50`})]}),(0,j.jsxs)(`div`,{className:`flex items-center gap-1.5`,children:[(0,j.jsx)(`button`,{onClick:()=>ke(e=>{let t=new Set(e);return t.has(n)?t.delete(n):t.add(n),t}),className:`flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground`,title:o(`3L8n3Q`,`Edit language overrides`),children:i(`LPrPc`,`{=m0} {=m1} {=m2} {=m3} {=m4} {=m5}`,{"=m0":(0,j.jsx)(ie,{size:10}),"=m1":(0,j.jsx)(`span`,{children:t.source_language||e.defaults?.source_language||`?`}),"=m2":(0,j.jsx)(`span`,{children:`→`}),"=m3":(0,j.jsx)(`span`,{children:(t.target_languages??e.defaults?.target_languages)?.join(`, `)||`?`}),"=m4":(t.source_language||t.target_languages)&&(0,j.jsx)(_,{variant:`secondary`,className:`ml-0.5 px-1 py-0 text-[9px]`,children:`override`}),"=m5":(0,j.jsx)(c,{size:8,className:`ml-0.5`})})}),(0,j.jsx)(`div`,{className:`opacity-0 group-hover:opacity-100`,children:(0,j.jsx)(y,{onDelete:()=>J(n),mode:`icon`})})]})]}),Oe.has(n)&&(0,j.jsx)(`div`,{className:`border-b border-border px-4 py-3`,children:(0,j.jsxs)(`div`,{className:`grid grid-cols-2 gap-3`,children:[(0,j.jsxs)(`div`,{children:[(0,j.jsx)(m,{className:`mb-0.5 block text-xs text-muted-foreground`,children:o(`22HW2L`,`Source override`)}),(0,j.jsx)(_e,{value:t.source_language??``,onChange:e=>q(n,{...t,source_language:e||void 0}),locales:b,placeholder:e.defaults?.source_language?o(`10wAUR`,`Inherit ({source})`,{source:e.defaults.source_language}):o(`4FMXin`,`Select source...`)})]}),(0,j.jsxs)(`div`,{children:[(0,j.jsx)(m,{className:`mb-0.5 block text-xs text-muted-foreground`,children:o(`3fg24Z`,`Target overrides`)}),(0,j.jsx)(ge,{value:t.target_languages??[],onChange:e=>q(n,{...t,target_languages:e.length>0?e:void 0}),locales:b,placeholder:e.defaults?.target_languages?.length?o(`s2EQV`,`Inherit ({targets})`,{targets:e.defaults.target_languages.join(`, `)}):o(`1YSqO`,`Add targets...`)})]})]})}),(0,j.jsx)(`div`,{className:`space-y-0 divide-y divide-border`,children:(t.items??[]).map((e,r)=>(0,j.jsxs)(`div`,{className:`group/item relative px-4 py-3`,children:[(0,j.jsx)(`div`,{className:`absolute right-2 top-2 opacity-0 group-hover/item:opacity-100`,children:(0,j.jsx)(y,{onDelete:()=>{let e=(t.items??[]).filter((e,t)=>t!==r);e.length===0?J(n):q(n,{...t,items:e})},mode:`icon`})}),$(e,e=>{let i=[...t.items??[]];i[r]=e,q(n,{...t,items:i})},`coll-${n}-${r}`)]},r))}),(0,j.jsx)(`div`,{className:`border-t border-border px-4 py-2`,children:(0,j.jsx)(h,{variant:`ghost`,size:`xs`,onClick:()=>q(n,{...t,items:[...t.items??[],{path:``}]}),className:`text-muted-foreground`,children:i(`1J6NlH`,`{=m0} Add another pattern`,{"=m0":(0,j.jsx)(l,{size:10})})})})]},n)})}):(0,j.jsxs)(`div`,{className:`rounded-xl border border-dashed border-border p-6 text-center`,children:[(0,j.jsx)(u,{size:20,className:`mx-auto mb-2 text-muted-foreground/50`}),(0,j.jsx)(`p`,{className:`text-sm text-muted-foreground`,children:o(`1LlEC5`,`No content patterns. Add a collection to map your source files.`)})]})]}),(0,j.jsxs)(`section`,{className:`flex min-h-0 flex-col overflow-auto`,children:[(0,j.jsxs)(`div`,{className:`mb-3 flex items-center justify-between`,children:[(0,j.jsx)(`h2`,{className:`flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground`,children:i(`YVw1j`,`Files {=m0}`,{"=m0":(0,j.jsxs)(`span`,{className:`text-xs font-normal`,children:[`(`,C.length,` matched`,!I&&Z.length>0&&`, ${Z.length} other`,Q>0&&` of ${Q} total`,`)`]})})}),(0,j.jsxs)(`div`,{className:`flex items-center gap-2`,children:[(0,j.jsxs)(h,{variant:`outline`,size:`sm`,onClick:()=>L(!I),className:I?`bg-accent`:``,"aria-label":I?o(`3N7kdg`,`Show all files`):o(`4syAql`,`Hide unmatched files`),title:I?o(`35tLxT`,`Show all files`):o(`2I66OW`,`Hide unmatched files`),children:[I?(0,j.jsx)(oe,{size:12}):(0,j.jsx)(se,{size:12}),I?o(`4AAIEq`,`Show all`):o(`3jYLun`,`Matched only`)]}),(0,j.jsx)(h,{variant:`outline`,size:`sm`,onClick:je,"aria-label":o(`2siFGL`,`Add files`),children:i(`3yaXiN`,`{=m0} Add Files`,{"=m0":(0,j.jsx)(l,{size:12})})}),(0,j.jsx)(h,{variant:`outline`,size:`icon-sm`,onClick:G,disabled:M,"aria-label":o(`17LbPZ`,`Rescan files`),children:M?(0,j.jsx)(ce,{size:12,className:`animate-spin`}):(0,j.jsx)(s,{size:12})})]})]}),(0,j.jsx)(`div`,{onDrop:Y,onDragOver:Me,onDragLeave:Ne,className:`min-h-[120px] rounded-lg border-2 transition-colors ${R?`border-primary bg-primary/5`:`border-transparent`}`,children:C.length===0&&(I||T.length===0)?(0,j.jsxs)(`div`,{className:`flex flex-col items-center justify-center py-12 text-center`,children:[(0,j.jsx)(ae,{size:24,className:`mb-3 text-muted-foreground/50`}),(0,j.jsx)(`p`,{className:`text-sm text-muted-foreground`,children:H.length>0?o(`4j98jS`,`No files matched the configured patterns.`):o(`2dAIJK`,`Drop files here or click Add Files to add them to the project.`)})]}):(0,j.jsxs)(`div`,{className:`space-y-4`,children:[Fe.map(e=>{let t=X.get(e)??[];return(0,j.jsxs)(`div`,{children:[e&&(0,j.jsx)(`h3`,{className:`mb-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground`,children:i(`1DBfJU`,`{collName} {=m1}`,{"=m1":(0,j.jsxs)(`span`,{className:`ml-1.5 font-normal`,children:[`(`,t.length,`)`]})},{collName:e})}),(0,j.jsx)(g,{children:(0,j.jsxs)(`table`,{className:`w-full text-xs`,children:[(0,j.jsx)(`thead`,{children:(0,j.jsxs)(`tr`,{className:`border-b border-border text-left text-muted-foreground`,children:[(0,j.jsx)(`th`,{className:`px-3 py-2 font-medium`,children:o(`S1hum`,`File`)}),(0,j.jsx)(`th`,{className:`px-3 py-2 font-medium`,children:o(`2cObl8`,`Format`)}),(0,j.jsx)(`th`,{className:`px-3 py-2 font-medium`,children:o(`257BUR`,`Pattern`)})]})}),(0,j.jsx)(`tbody`,{children:t.map((e,t)=>(0,j.jsxs)(`tr`,{className:`border-b border-border last:border-0 hover:bg-accent/30`,children:[(0,j.jsx)(`td`,{className:`px-3 py-1.5`,children:(0,j.jsxs)(`span`,{className:`flex items-center gap-1.5 font-mono`,children:[(0,j.jsx)(u,{size:12,className:`shrink-0 text-muted-foreground`}),e.relative]})}),(0,j.jsx)(`td`,{className:`px-3 py-1.5`,children:(0,j.jsx)(_,{variant:`secondary`,children:e.format||`unknown`})}),(0,j.jsx)(`td`,{className:`px-3 py-1.5 text-muted-foreground`,children:e.pattern})]},t))})]})})]},e||`__uncollected`)}),!I&&Z.length>0&&(0,j.jsxs)(`div`,{children:[C.length>0&&(0,j.jsx)(`h3`,{className:`mb-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground`,children:i(`4cZB4u`,`Other files {=m0}`,{"=m0":(0,j.jsxs)(`span`,{className:`ml-1.5 font-normal`,children:[`(`,Z.length,`)`]})})}),(0,j.jsx)(g,{children:(0,j.jsxs)(`table`,{className:`w-full text-xs`,children:[(0,j.jsx)(`thead`,{children:(0,j.jsxs)(`tr`,{className:`border-b border-border text-left text-muted-foreground`,children:[(0,j.jsx)(`th`,{className:`px-3 py-2 font-medium`,children:o(`S1hum`,`File`)}),(0,j.jsx)(`th`,{className:`px-3 py-2 font-medium`,children:o(`2cObl8`,`Format`)}),(0,j.jsx)(`th`,{className:`px-3 py-2 text-right font-medium`,children:o(`D1JM0`,`Size`)})]})}),(0,j.jsx)(`tbody`,{children:Z.map(e=>(0,j.jsxs)(`tr`,{className:`border-b border-border last:border-0 text-muted-foreground hover:bg-accent/30`,children:[(0,j.jsx)(`td`,{className:`px-3 py-1.5`,children:(0,j.jsxs)(`span`,{className:`flex items-center gap-1.5 font-mono`,children:[(0,j.jsx)(u,{size:12,className:`shrink-0`}),e.relative]})}),(0,j.jsx)(`td`,{className:`px-3 py-1.5`,children:e.format?(0,j.jsx)(_,{variant:`secondary`,children:e.format}):(0,j.jsx)(`span`,{children:o(`313IoL`,`—`)})}),(0,j.jsx)(`td`,{className:`px-3 py-1.5 text-right`,children:Te(e.size)})]},e.relative))})]})})]})]})})]})]})]})}var A,j,M=t((()=>{a(),A=e(n(),1),d(),p(),w(),x(),T(),b(),C(),E(),D(),j=r(),k.__docgenInfo={description:``,methods:[],displayName:`ContentPage`,props:{project:{required:!0,tsType:{name:`KapiProject`},description:``},projectPath:{required:!0,tsType:{name:`string`},description:``},onUpdate:{required:!0,tsType:{name:`signature`,type:`function`,raw:`(project: KapiProject) => void`,signature:{arguments:[{type:{name:`KapiProject`},name:`project`}],return:{name:`void`}}},description:``},tabID:{required:!0,tsType:{name:`string`},description:``},formatList:{required:!1,tsType:{name:`Array`,elements:[{name:`FormatInfo`}],raw:`FormatInfo[]`},description:`Pre-loaded formats for Storybook — skips api.listFormats().`},basePath:{required:!1,tsType:{name:`string`},description:`Pre-loaded base path for Storybook — skips api.getBasePath().`}}}})),N,P,F,I,L,R,z,B;t((()=>{M(),{fn:N}=__STORYBOOK_MODULE_TEST__,P={title:`Pages/ContentPage`,component:k,tags:[`autodocs`],args:{onUpdate:N(),tabID:`tab-1`,projectPath:`/Users/dev/acme-app/project.kapi`,formatList:[{name:`json`,display_name:`JSON`,extensions:[`.json`],has_reader:!0,has_writer:!0,has_schema:!1},{name:`xliff`,display_name:`XLIFF 1.2`,extensions:[`.xlf`,`.xliff`],has_reader:!0,has_writer:!0,has_schema:!1},{name:`xliff2`,display_name:`XLIFF 2.0`,extensions:[`.xlf`],has_reader:!0,has_writer:!0,has_schema:!1},{name:`po`,display_name:`Gettext PO`,extensions:[`.po`,`.pot`],has_reader:!0,has_writer:!0,has_schema:!1},{name:`properties`,display_name:`Java Properties`,extensions:[`.properties`],has_reader:!0,has_writer:!0,has_schema:!1},{name:`markdown`,display_name:`Markdown`,extensions:[`.md`],has_reader:!0,has_writer:!0,has_schema:!1},{name:`html`,display_name:`HTML`,extensions:[`.html`,`.htm`],has_reader:!0,has_writer:!0,has_schema:!1},{name:`xml`,display_name:`XML`,extensions:[`.xml`],has_reader:!0,has_writer:!0,has_schema:!1},{name:`csv`,display_name:`CSV`,extensions:[`.csv`],has_reader:!0,has_writer:!0,has_schema:!1},{name:`yaml`,display_name:`YAML`,extensions:[`.yaml`,`.yml`],has_reader:!0,has_writer:!0,has_schema:!1},{name:`okf_html`,display_name:`HTML (Okapi)`,extensions:[`.html`],has_reader:!0,has_writer:!0,source:`okapi`,has_schema:!0},{name:`okf_xml`,display_name:`XML (Okapi)`,extensions:[`.xml`],has_reader:!0,has_writer:!0,source:`okapi`,has_schema:!0}],basePath:`/Users/dev/acme-app`}},F={args:{project:{version:`v1`,name:`Acme App Localization`,defaults:{source_language:`en-US`,target_languages:[`fr-FR`,`de-DE`]},preset:`nextjs`,plugins:{okapi:{framework_version:`^1.47.0`,format_priority:200}},content:[{path:`src/i18n/en/*.json`,format:{name:`json`},target:`src/i18n/{lang}/*.json`},{path:`docs/en/**/*.md`,format:{name:`markdown`}}]}}},I={args:{project:{version:`v1`,name:`Multi-Collection Project`,defaults:{source_language:`en-US`,target_languages:[`fr-FR`,`de-DE`,`ja-JP`],formats:{okf_html:{preset:`strict-extraction`}}},plugins:{okapi:{framework_version:`^1.47.0`,format_priority:200}},content:[{path:`src/i18n/en/*.json`,target:`src/i18n/{lang}/*.json`},{name:`Marketing`,target_languages:[`fr-FR`,`de-DE`],items:[{path:`marketing/**/*.html`,format:{name:`okf_html`,preset:`lenient`},target:`marketing/{lang}/**/*.html`},{path:`marketing/**/*.json`,target:`marketing/{lang}/**/*.json`}]},{name:`China`,source_language:`zh-CN`,target_languages:[`en-US`],items:[{path:`china/**/*`,target:`china/output/{lang}/**/*`}]}]}}},L={args:{project:{version:`v1`,name:`Okapi Bridge Project`,defaults:{source_language:`en-US`,target_languages:[`fr-FR`,`ja-JP`]},plugins:{okapi:{framework_version:`^1.47.0`}},content:[{name:`Documentation`,items:[{path:`docs/**/*.html`,format:{name:`okf_html`,preset:`strict-extraction`},target:`output/{lang}/docs/**/*.html`}]},{name:`Emails`,items:[{path:`emails/*.html`,format:{name:`okf_html`,config:{useCodeFinder:!0,escapeGT:!1}}}]},{path:`src/i18n/en/*.json`,target:`src/i18n/{lang}/*.json`}]}}},R={args:{project:{version:`v1`,name:`Pinned Plugins`,defaults:{source_language:`en-US`,target_languages:[`fr-FR`]},plugins:{okapi:{version:`^0.38.0`,framework_version:`^1.47.0`,format_priority:200},"custom-filter":{version:`^2.1.0`}},content:[{path:`input/*`,format:{name:`okf_html`},target:`output/{lang}/*`}]}}},z={args:{project:{version:`v1`,name:`New Project`,defaults:{source_language:`en`,target_languages:[`fr-FR`]},content:[]}}},F.parameters={...F.parameters,docs:{...F.parameters?.docs,source:{originalSource:`{
  args: {
    project: {
      version: "v1",
      name: "Acme App Localization",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR", "de-DE"]
      },
      preset: "nextjs",
      plugins: {
        okapi: {
          framework_version: "^1.47.0",
          format_priority: 200
        }
      },
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
      }]
    }
  }
}`,...F.parameters?.docs?.source}}},I.parameters={...I.parameters,docs:{...I.parameters?.docs,source:{originalSource:`{
  args: {
    project: {
      version: "v1",
      name: "Multi-Collection Project",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR", "de-DE", "ja-JP"],
        formats: {
          okf_html: {
            preset: "strict-extraction"
          }
        }
      },
      plugins: {
        okapi: {
          framework_version: "^1.47.0",
          format_priority: 200
        }
      },
      content: [{
        path: "src/i18n/en/*.json",
        target: "src/i18n/{lang}/*.json"
      }, {
        name: "Marketing",
        target_languages: ["fr-FR", "de-DE"],
        items: [{
          path: "marketing/**/*.html",
          format: {
            name: "okf_html",
            preset: "lenient"
          },
          target: "marketing/{lang}/**/*.html"
        }, {
          path: "marketing/**/*.json",
          target: "marketing/{lang}/**/*.json"
        }]
      }, {
        name: "China",
        source_language: "zh-CN",
        target_languages: ["en-US"],
        items: [{
          path: "china/**/*",
          target: "china/output/{lang}/**/*"
        }]
      }]
    }
  }
}`,...I.parameters?.docs?.source}}},L.parameters={...L.parameters,docs:{...L.parameters?.docs,source:{originalSource:`{
  args: {
    project: {
      version: "v1",
      name: "Okapi Bridge Project",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR", "ja-JP"]
      },
      plugins: {
        okapi: {
          framework_version: "^1.47.0"
        }
      },
      content: [{
        name: "Documentation",
        items: [{
          path: "docs/**/*.html",
          format: {
            name: "okf_html",
            preset: "strict-extraction"
          },
          target: "output/{lang}/docs/**/*.html"
        }]
      }, {
        name: "Emails",
        items: [{
          path: "emails/*.html",
          format: {
            name: "okf_html",
            config: {
              useCodeFinder: true,
              escapeGT: false
            }
          }
        }]
      }, {
        path: "src/i18n/en/*.json",
        target: "src/i18n/{lang}/*.json"
      }]
    }
  }
}`,...L.parameters?.docs?.source}}},R.parameters={...R.parameters,docs:{...R.parameters?.docs,source:{originalSource:`{
  args: {
    project: {
      version: "v1",
      name: "Pinned Plugins",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR"]
      },
      plugins: {
        okapi: {
          version: "^0.38.0",
          framework_version: "^1.47.0",
          format_priority: 200
        },
        "custom-filter": {
          version: "^2.1.0"
        }
      },
      content: [{
        path: "input/*",
        format: {
          name: "okf_html"
        },
        target: "output/{lang}/*"
      }]
    }
  }
}`,...R.parameters?.docs?.source}}},z.parameters={...z.parameters,docs:{...z.parameters?.docs,source:{originalSource:`{
  args: {
    project: {
      version: "v1",
      name: "New Project",
      defaults: {
        source_language: "en",
        target_languages: ["fr-FR"]
      },
      content: []
    }
  }
}`,...z.parameters?.docs?.source}}},B=[`Default`,`WithCollections`,`WithFormatPresets`,`WithPlugins`,`Empty`]}))();export{F as Default,z as Empty,I as WithCollections,L as WithFormatPresets,R as WithPlugins,B as __namedExportsOrder,P as default};