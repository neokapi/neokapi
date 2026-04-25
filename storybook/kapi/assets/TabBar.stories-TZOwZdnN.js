import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{r as o,t as s}from"./lucide-react-CNTu2eXS.js";import{t as c}from"./src-B5GKo4pn.js";import{t as l}from"./button-Dd5Xmast.js";function u({tabs:e,activeTabID:t,onSelect:n,onClose:r,onRename:i}){let[s,c]=(0,d.useState)(null),[u,p]=(0,d.useState)(``),m=(0,d.useRef)(null);(0,d.useEffect)(()=>{s&&m.current&&(m.current.focus(),m.current.select())},[s]);let h=e=>{c(e.id),p(e.name)},g=()=>{s&&u.trim()&&i(s,u.trim()),c(null)};return e.length===0?null:(0,f.jsx)(`div`,{className:`flex items-end gap-px overflow-x-auto px-1`,children:e.map(e=>(0,f.jsxs)(`div`,{onClick:()=>n(e.id),className:`group flex min-w-[160px] max-w-[240px] cursor-pointer items-center justify-between gap-1.5 rounded-t-lg px-3 py-1.5 text-xs transition-all ${t===e.id?`bg-border text-foreground font-semibold`:`text-muted-foreground hover:bg-accent/40 hover:text-foreground`}`,children:[s===e.id?(0,f.jsx)(`input`,{ref:m,value:u,onChange:e=>p(e.target.value),onBlur:g,onKeyDown:e=>{e.key===`Enter`&&g(),e.key===`Escape`&&c(null)},className:`w-24 rounded bg-transparent px-0.5 text-xs outline-none ring-1 ring-ring`,"aria-label":a(`Rfyk7`,`Rename project`)}):(0,f.jsx)(`span`,{onDoubleClick:()=>h(e),className:`max-w-[140px] truncate`,title:e.path?`${e.name} — ${e.path}`:e.name,children:e.name}),(0,f.jsx)(l,{variant:`ghost`,size:`icon-xs`,onClick:t=>{t.stopPropagation(),r(e.id)},className:`h-4 w-4 opacity-0 group-hover:opacity-100`,"aria-label":a(`4bRHns`,`Close {name}`,{name:e.name}),children:(0,f.jsx)(o,{size:10})})]},e.id))})}var d,f,p=t((()=>{i(),d=e(n(),1),s(),c(),f=r(),u.__docgenInfo={description:``,methods:[],displayName:`TabBar`,props:{tabs:{required:!0,tsType:{name:`Array`,elements:[{name:`TabInfo`}],raw:`TabInfo[]`},description:``},activeTabID:{required:!0,tsType:{name:`union`,raw:`string | null`,elements:[{name:`string`},{name:`null`}]},description:``},onSelect:{required:!0,tsType:{name:`signature`,type:`function`,raw:`(tabID: string) => void`,signature:{arguments:[{type:{name:`string`},name:`tabID`}],return:{name:`void`}}},description:``},onClose:{required:!0,tsType:{name:`signature`,type:`function`,raw:`(tabID: string) => void`,signature:{arguments:[{type:{name:`string`},name:`tabID`}],return:{name:`void`}}},description:``},onRename:{required:!0,tsType:{name:`signature`,type:`function`,raw:`(tabID: string, name: string) => void`,signature:{arguments:[{type:{name:`string`},name:`tabID`},{type:{name:`string`},name:`name`}],return:{name:`void`}}},description:``}}}})),m,h,g,_,v,y,b,x;t((()=>{p(),m=r(),{fn:h}=__STORYBOOK_MODULE_TEST__,g={title:`Components/TabBar`,component:u,tags:[`autodocs`],args:{onSelect:h(),onClose:h()},decorators:[e=>(0,m.jsx)(`div`,{style:{width:800},children:(0,m.jsx)(e,{})})]},_={args:{tabs:[{id:`1`,name:`translation.kapi`,path:`/Users/dev/translation.kapi`}],activeTabID:`1`}},v={args:{tabs:[{id:`1`,name:`translation.kapi`,path:`/Users/dev/translation.kapi`},{id:`2`,name:`qa-pipeline.kapi`,path:`/Users/dev/qa-pipeline.kapi`},{id:`3`,name:`New Project`,path:``}],activeTabID:`2`}},y={args:{tabs:Array.from({length:8},(e,t)=>({id:String(t+1),name:`project-${t+1}.kapi`,path:`/tmp/project-${t+1}.kapi`})),activeTabID:`3`}},b={args:{tabs:[],activeTabID:null}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  args: {
    tabs: [{
      id: "1",
      name: "translation.kapi",
      path: "/Users/dev/translation.kapi"
    }],
    activeTabID: "1"
  }
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  args: {
    tabs: [{
      id: "1",
      name: "translation.kapi",
      path: "/Users/dev/translation.kapi"
    }, {
      id: "2",
      name: "qa-pipeline.kapi",
      path: "/Users/dev/qa-pipeline.kapi"
    }, {
      id: "3",
      name: "New Project",
      path: ""
    }],
    activeTabID: "2"
  }
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  args: {
    tabs: Array.from({
      length: 8
    }, (_, i) => ({
      id: String(i + 1),
      name: \`project-\${i + 1}.kapi\`,
      path: \`/tmp/project-\${i + 1}.kapi\`
    })),
    activeTabID: "3"
  }
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  args: {
    tabs: [],
    activeTabID: null
  }
}`,...b.parameters?.docs?.source}}},x=[`SingleTab`,`MultipleTabs`,`ManyTabs`,`NoTabs`]}))();export{y as ManyTabs,v as MultipleTabs,b as NoTabs,_ as SingleTab,x as __namedExportsOrder,g as default};