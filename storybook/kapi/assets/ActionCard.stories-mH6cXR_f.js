import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,r,t as i}from"./runtime-B587fM_0.js";import{S as a,a as o,et as s,h as c,i as l,ot as u,rt as d,t as f,ut as p}from"./lucide-react-CNTu2eXS.js";import{n as m,t as h}from"./badge-DDhlaGcs.js";import{n as g,t as _}from"./action-card-BjEy4xdK.js";var v,y,b,x,S,C,w,T;e((()=>{r(),g(),m(),f(),v=t(),y={title:`Foundations/ActionCard`,component:_,tags:[`autodocs`],parameters:{docs:{description:{component:`Clickable card for templates, presets, and quick actions. Supports icon, title, description, badge, loading, and highlighted states.`}}}},b={name:`Template Card`,render:()=>(0,v.jsxs)(`div`,{className:`max-w-lg space-y-3`,children:[(0,v.jsx)(_,{icon:(0,v.jsx)(`div`,{className:`flex items-center gap-1.5`,children:n(`3Pnp9T`,`{=m0} {=m1} {=m2}`,{"=m0":(0,v.jsx)(d,{size:20}),"=m1":(0,v.jsx)(`span`,{className:`text-xs text-muted-foreground`,children:`→`}),"=m2":(0,v.jsx)(s,{size:20})})}),title:i(`3QavRZ`,`Input → Output`),description:i(`2khCUA`,`Source files in ./input/, translations written to ./output/{lang}/.`),onClick:()=>{}}),(0,v.jsx)(_,{icon:(0,v.jsx)(p,{size:20}),title:i(`3vMRIa`,`Start empty`),description:i(`4nsYDP`,`Blank project — configure everything yourself.`),onClick:()=>{}})]})},x={name:`Preset Cards (with highlighted)`,render:()=>(0,v.jsxs)(`div`,{className:`max-w-lg space-y-3`,children:[(0,v.jsx)(_,{icon:(0,v.jsx)(c,{size:20}),title:i(`490peS`,`nextjs`),description:i(`SKnGy`,`Next.js App Router with next-intl`),badge:(0,v.jsx)(h,{variant:`secondary`,className:`text-[10px]`,children:i(`1qGHMS`,`detected`)}),highlighted:!0,onClick:()=>{}}),(0,v.jsx)(_,{icon:(0,v.jsx)(c,{size:20}),title:i(`479xUT`,`react-intl`),description:i(`Iqon8`,`React with react-intl (FormatJS)`),onClick:()=>{}}),(0,v.jsx)(_,{icon:(0,v.jsx)(c,{size:20}),title:i(`4z6eHd`,`angular`),description:i(`2Ad42r`,`Angular with @angular/localize`),onClick:()=>{}})]})},S={name:`Quick Action Grid`,render:()=>(0,v.jsxs)(`div`,{className:`grid max-w-2xl grid-cols-2 gap-3 lg:grid-cols-4`,children:[(0,v.jsx)(_,{icon:(0,v.jsx)(u,{size:16}),title:i(`24vTiJ`,`Content`),description:i(`3SKWTz`,`2 collections, 4 patterns`),onClick:()=>{}}),(0,v.jsx)(_,{icon:(0,v.jsx)(o,{size:16}),title:i(`3jRyCD`,`Flows`),description:i(`PeGXX`,`3 flows defined`),onClick:()=>{}}),(0,v.jsx)(_,{icon:(0,v.jsx)(l,{size:16}),title:i(`4BDNuN`,`Tools`),description:i(`1VduUZ`,`Run individual tools`),onClick:()=>{}}),(0,v.jsx)(_,{icon:(0,v.jsx)(a,{size:16}),title:i(`1GBVV`,`Settings`),description:i(`44AwwJ`,`Languages, plugins`),onClick:()=>{}})]})},C={name:`Loading State`,args:{icon:(0,v.jsx)(c,{size:20}),title:`Applying preset...`,description:`Setting up project configuration.`,loading:!0,onClick:()=>{}}},w={name:`Disabled State`,args:{icon:(0,v.jsx)(p,{size:20}),title:`Unavailable`,description:`This option is not available.`,disabled:!0,onClick:()=>{}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  name: "Template Card",
  render: () => <div className="max-w-lg space-y-3">
      <ActionCard icon={<div className="flex items-center gap-1.5">
            <FolderInput size={20} />
            <span className="text-xs text-muted-foreground">&rarr;</span>
            <FolderOutput size={20} />
          </div>} title="Input → Output" description="Source files in ./input/, translations written to ./output/{lang}/." onClick={() => {}} />
      <ActionCard icon={<FileBox size={20} />} title="Start empty" description="Blank project — configure everything yourself." onClick={() => {}} />
    </div>
}`,...b.parameters?.docs?.source}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  name: "Preset Cards (with highlighted)",
  render: () => <div className="max-w-lg space-y-3">
      <ActionCard icon={<Sparkles size={20} />} title="nextjs" description="Next.js App Router with next-intl" badge={<Badge variant="secondary" className="text-[10px]">
            detected
          </Badge>} highlighted onClick={() => {}} />
      <ActionCard icon={<Sparkles size={20} />} title="react-intl" description="React with react-intl (FormatJS)" onClick={() => {}} />
      <ActionCard icon={<Sparkles size={20} />} title="angular" description="Angular with @angular/localize" onClick={() => {}} />
    </div>
}`,...x.parameters?.docs?.source}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  name: "Quick Action Grid",
  render: () => <div className="grid max-w-2xl grid-cols-2 gap-3 lg:grid-cols-4">
      <ActionCard icon={<FileText size={16} />} title="Content" description="2 collections, 4 patterns" onClick={() => {}} />
      <ActionCard icon={<Workflow size={16} />} title="Flows" description="3 flows defined" onClick={() => {}} />
      <ActionCard icon={<Wrench size={16} />} title="Tools" description="Run individual tools" onClick={() => {}} />
      <ActionCard icon={<Settings2 size={16} />} title="Settings" description="Languages, plugins" onClick={() => {}} />
    </div>
}`,...S.parameters?.docs?.source}}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  name: "Loading State",
  args: {
    icon: <Sparkles size={20} />,
    title: "Applying preset...",
    description: "Setting up project configuration.",
    loading: true,
    onClick: () => {}
  }
}`,...C.parameters?.docs?.source}}},w.parameters={...w.parameters,docs:{...w.parameters?.docs,source:{originalSource:`{
  name: "Disabled State",
  args: {
    icon: <FileBox size={20} />,
    title: "Unavailable",
    description: "This option is not available.",
    disabled: true,
    onClick: () => {}
  }
}`,...w.parameters?.docs?.source}}},T=[`Template`,`PresetCards`,`QuickActions`,`Loading`,`Disabled`]}))();export{w as Disabled,C as Loading,x as PresetCards,S as QuickActions,b as Template,T as __namedExportsOrder,y as default};