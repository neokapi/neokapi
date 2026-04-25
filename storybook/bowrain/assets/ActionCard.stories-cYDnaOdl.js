import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{T as n,a as r,b as i,bt as a,ct as o,i as s,mt as c,t as l,ut as u}from"./lucide-react-CMMmtkYs.js";import{n as d,t as f}from"./badge-CyOz_y6s.js";import{n as p,t as m}from"./action-card-Cw00J01D.js";var h,g,_,v,y,b,x,S;e((()=>{p(),d(),l(),h=t(),g={title:`Foundations/ActionCard`,component:m,tags:[`autodocs`],parameters:{docs:{description:{component:`Clickable card for templates, presets, and quick actions. Supports icon, title, description, badge, loading, and highlighted states.`}}}},_={name:`Template Card`,render:()=>(0,h.jsxs)(`div`,{className:`max-w-lg space-y-3`,children:[(0,h.jsx)(m,{icon:(0,h.jsxs)(`div`,{className:`flex items-center gap-1.5`,children:[(0,h.jsx)(u,{size:20}),(0,h.jsx)(`span`,{className:`text-xs text-muted-foreground`,children:`→`}),(0,h.jsx)(o,{size:20})]}),title:`Input → Output`,description:`Source files in ./input/, translations written to ./output/{lang}/.`,onClick:()=>{}}),(0,h.jsx)(m,{icon:(0,h.jsx)(a,{size:20}),title:`Start empty`,description:`Blank project — configure everything yourself.`,onClick:()=>{}})]})},v={name:`Preset Cards (with highlighted)`,render:()=>(0,h.jsxs)(`div`,{className:`max-w-lg space-y-3`,children:[(0,h.jsx)(m,{icon:(0,h.jsx)(i,{size:20}),title:`nextjs`,description:`Next.js App Router with next-intl`,badge:(0,h.jsx)(f,{variant:`secondary`,className:`text-[10px]`,children:`detected`}),highlighted:!0,onClick:()=>{}}),(0,h.jsx)(m,{icon:(0,h.jsx)(i,{size:20}),title:`react-intl`,description:`React with react-intl (FormatJS)`,onClick:()=>{}}),(0,h.jsx)(m,{icon:(0,h.jsx)(i,{size:20}),title:`angular`,description:`Angular with @angular/localize`,onClick:()=>{}})]})},y={name:`Quick Action Grid`,render:()=>(0,h.jsxs)(`div`,{className:`grid max-w-2xl grid-cols-2 gap-3 lg:grid-cols-4`,children:[(0,h.jsx)(m,{icon:(0,h.jsx)(c,{size:16}),title:`Content`,description:`2 collections, 4 patterns`,onClick:()=>{}}),(0,h.jsx)(m,{icon:(0,h.jsx)(r,{size:16}),title:`Flows`,description:`3 flows defined`,onClick:()=>{}}),(0,h.jsx)(m,{icon:(0,h.jsx)(s,{size:16}),title:`Tools`,description:`Run individual tools`,onClick:()=>{}}),(0,h.jsx)(m,{icon:(0,h.jsx)(n,{size:16}),title:`Settings`,description:`Languages, plugins`,onClick:()=>{}})]})},b={name:`Loading State`,args:{icon:(0,h.jsx)(i,{size:20}),title:`Applying preset...`,description:`Setting up project configuration.`,loading:!0,onClick:()=>{}}},x={name:`Disabled State`,args:{icon:(0,h.jsx)(a,{size:20}),title:`Unavailable`,description:`This option is not available.`,disabled:!0,onClick:()=>{}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "Template Card",
  render: () => <div className="max-w-lg space-y-3">
      <ActionCard icon={<div className="flex items-center gap-1.5">
            <FolderInput size={20} />
            <span className="text-xs text-muted-foreground">&rarr;</span>
            <FolderOutput size={20} />
          </div>} title="Input → Output" description="Source files in ./input/, translations written to ./output/{lang}/." onClick={() => {}} />
      <ActionCard icon={<FileBox size={20} />} title="Start empty" description="Blank project — configure everything yourself." onClick={() => {}} />
    </div>
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  name: "Preset Cards (with highlighted)",
  render: () => <div className="max-w-lg space-y-3">
      <ActionCard icon={<Sparkles size={20} />} title="nextjs" description="Next.js App Router with next-intl" badge={<Badge variant="secondary" className="text-[10px]">
            detected
          </Badge>} highlighted onClick={() => {}} />
      <ActionCard icon={<Sparkles size={20} />} title="react-intl" description="React with react-intl (FormatJS)" onClick={() => {}} />
      <ActionCard icon={<Sparkles size={20} />} title="angular" description="Angular with @angular/localize" onClick={() => {}} />
    </div>
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  name: "Quick Action Grid",
  render: () => <div className="grid max-w-2xl grid-cols-2 gap-3 lg:grid-cols-4">
      <ActionCard icon={<FileText size={16} />} title="Content" description="2 collections, 4 patterns" onClick={() => {}} />
      <ActionCard icon={<Workflow size={16} />} title="Flows" description="3 flows defined" onClick={() => {}} />
      <ActionCard icon={<Wrench size={16} />} title="Tools" description="Run individual tools" onClick={() => {}} />
      <ActionCard icon={<Settings2 size={16} />} title="Settings" description="Languages, plugins" onClick={() => {}} />
    </div>
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  name: "Loading State",
  args: {
    icon: <Sparkles size={20} />,
    title: "Applying preset...",
    description: "Setting up project configuration.",
    loading: true,
    onClick: () => {}
  }
}`,...b.parameters?.docs?.source}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  name: "Disabled State",
  args: {
    icon: <FileBox size={20} />,
    title: "Unavailable",
    description: "This option is not available.",
    disabled: true,
    onClick: () => {}
  }
}`,...x.parameters?.docs?.source}}},S=[`Template`,`PresetCards`,`QuickActions`,`Loading`,`Disabled`]}))();export{x as Disabled,b as Loading,v as PresetCards,y as QuickActions,_ as Template,S as __namedExportsOrder,g as default};