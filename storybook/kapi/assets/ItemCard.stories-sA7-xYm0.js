import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,r,t as i}from"./runtime-B587fM_0.js";import{a,ot as o,t as s}from"./lucide-react-CNTu2eXS.js";import{n as c,t as l}from"./badge-DDhlaGcs.js";import{n as u,t as d}from"./item-card-DGYr6ylt.js";import{n as f,t as p}from"./confirm-delete-button-C4rBI4_z.js";var m,h,g,_,v,y;e((()=>{r(),u(),f(),c(),s(),m=t(),h={title:`Foundations/ItemCard`,component:d,tags:[`autodocs`],parameters:{docs:{description:{component:`Universal card for list items. Consistent padding, hover, selection, and group behavior. Built on shadcn Card.`}}}},g={render:()=>(0,m.jsxs)(`div`,{className:`max-w-md space-y-2`,children:[(0,m.jsxs)(d,{children:[(0,m.jsxs)(`div`,{className:`flex items-center justify-between`,children:[(0,m.jsx)(`div`,{className:`flex items-center gap-2`,children:n(`1S5fXg`,`{=m0} {=m1}`,{"=m0":(0,m.jsx)(o,{size:14,className:`text-primary`}),"=m1":(0,m.jsx)(`span`,{className:`text-sm font-medium`,children:`src/i18n/en/*.json`})})}),(0,m.jsx)(p,{onDelete:()=>{},mode:`icon`})]}),(0,m.jsx)(`p`,{className:`mt-1 text-xs text-muted-foreground`,children:i(`4zhFqG`,`Target: src/i18n/{lang}/*.json`)})]}),(0,m.jsx)(d,{children:(0,m.jsxs)(`div`,{className:`flex items-center justify-between`,children:[(0,m.jsx)(`div`,{className:`flex items-center gap-2`,children:n(`1S5fXg`,`{=m0} {=m1}`,{"=m0":(0,m.jsx)(a,{size:14,className:`text-primary`}),"=m1":(0,m.jsx)(`span`,{className:`text-sm font-medium`,children:`translate-and-qa`})})}),(0,m.jsx)(l,{variant:`secondary`,className:`text-[10px]`,children:i(`1mCOcD`,`2 steps`)})]})})]})},_={render:()=>(0,m.jsx)(`div`,{className:`max-w-md`,children:(0,m.jsxs)(d,{clickable:!0,onClick:()=>alert(`Clicked!`),children:[(0,m.jsx)(`span`,{className:`text-sm font-medium`,children:i(`2H2oll`,`Click me`)}),(0,m.jsx)(`p`,{className:`text-xs text-muted-foreground`,children:i(`19ApZ4`,`This card has a pointer cursor and hover border.`)})]})})},v={render:()=>(0,m.jsx)(`div`,{className:`max-w-md`,children:(0,m.jsxs)(d,{selected:!0,children:[(0,m.jsx)(`span`,{className:`text-sm font-medium`,children:i(`1KIfhU`,`Selected card`)}),(0,m.jsx)(`p`,{className:`text-xs text-muted-foreground`,children:i(`2ydWC7`,`Primary border and subtle background.`)})]})})},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  render: () => <div className="max-w-md space-y-2">
      <ItemCard>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <FileText size={14} className="text-primary" />
            <span className="text-sm font-medium">src/i18n/en/*.json</span>
          </div>
          <ConfirmDeleteButton onDelete={() => {}} mode="icon" />
        </div>
        <p className="mt-1 text-xs text-muted-foreground">
          Target: src/i18n/&#123;lang&#125;/*.json
        </p>
      </ItemCard>
      <ItemCard>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Workflow size={14} className="text-primary" />
            <span className="text-sm font-medium">translate-and-qa</span>
          </div>
          <Badge variant="secondary" className="text-[10px]">
            2 steps
          </Badge>
        </div>
      </ItemCard>
    </div>
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  render: () => <div className="max-w-md">
      <ItemCard clickable onClick={() => alert("Clicked!")}>
        <span className="text-sm font-medium">Click me</span>
        <p className="text-xs text-muted-foreground">
          This card has a pointer cursor and hover border.
        </p>
      </ItemCard>
    </div>
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  render: () => <div className="max-w-md">
      <ItemCard selected>
        <span className="text-sm font-medium">Selected card</span>
        <p className="text-xs text-muted-foreground">Primary border and subtle background.</p>
      </ItemCard>
    </div>
}`,...v.parameters?.docs?.source}}},y=[`Default`,`Clickable`,`Selected`]}))();export{_ as Clickable,g as Default,v as Selected,y as __namedExportsOrder,h as default};