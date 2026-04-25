import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{a as n,mt as r,t as i}from"./lucide-react-CMMmtkYs.js";import{n as a,t as o}from"./badge-CyOz_y6s.js";import{n as s,t as c}from"./item-card-DBMzUU7s.js";import{n as l,t as u}from"./confirm-delete-button-Byh0BHlp.js";var d,f,p,m,h,g;e((()=>{s(),l(),a(),i(),d=t(),f={title:`Foundations/ItemCard`,component:c,tags:[`autodocs`],parameters:{docs:{description:{component:`Universal card for list items. Consistent padding, hover, selection, and group behavior. Built on shadcn Card.`}}}},p={render:()=>(0,d.jsxs)(`div`,{className:`max-w-md space-y-2`,children:[(0,d.jsxs)(c,{children:[(0,d.jsxs)(`div`,{className:`flex items-center justify-between`,children:[(0,d.jsxs)(`div`,{className:`flex items-center gap-2`,children:[(0,d.jsx)(r,{size:14,className:`text-primary`}),(0,d.jsx)(`span`,{className:`text-sm font-medium`,children:`src/i18n/en/*.json`})]}),(0,d.jsx)(u,{onDelete:()=>{},mode:`icon`})]}),(0,d.jsx)(`p`,{className:`mt-1 text-xs text-muted-foreground`,children:`Target: src/i18n/{lang}/*.json`})]}),(0,d.jsx)(c,{children:(0,d.jsxs)(`div`,{className:`flex items-center justify-between`,children:[(0,d.jsxs)(`div`,{className:`flex items-center gap-2`,children:[(0,d.jsx)(n,{size:14,className:`text-primary`}),(0,d.jsx)(`span`,{className:`text-sm font-medium`,children:`translate-and-qa`})]}),(0,d.jsx)(o,{variant:`secondary`,className:`text-[10px]`,children:`2 steps`})]})})]})},m={render:()=>(0,d.jsx)(`div`,{className:`max-w-md`,children:(0,d.jsxs)(c,{clickable:!0,onClick:()=>alert(`Clicked!`),children:[(0,d.jsx)(`span`,{className:`text-sm font-medium`,children:`Click me`}),(0,d.jsx)(`p`,{className:`text-xs text-muted-foreground`,children:`This card has a pointer cursor and hover border.`})]})})},h={render:()=>(0,d.jsx)(`div`,{className:`max-w-md`,children:(0,d.jsxs)(c,{selected:!0,children:[(0,d.jsx)(`span`,{className:`text-sm font-medium`,children:`Selected card`}),(0,d.jsx)(`p`,{className:`text-xs text-muted-foreground`,children:`Primary border and subtle background.`})]})})},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
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
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => <div className="max-w-md">
      <ItemCard clickable onClick={() => alert("Clicked!")}>
        <span className="text-sm font-medium">Click me</span>
        <p className="text-xs text-muted-foreground">
          This card has a pointer cursor and hover border.
        </p>
      </ItemCard>
    </div>
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  render: () => <div className="max-w-md">
      <ItemCard selected>
        <span className="text-sm font-medium">Selected card</span>
        <p className="text-xs text-muted-foreground">Primary border and subtle background.</p>
      </ItemCard>
    </div>
}`,...h.parameters?.docs?.source}}},g=[`Default`,`Clickable`,`Selected`]}))();export{m as Clickable,p as Default,h as Selected,g as __namedExportsOrder,f as default};