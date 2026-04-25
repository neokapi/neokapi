import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{r as n,t as r}from"./runtime-B587fM_0.js";import{n as i,t as a}from"./confirm-delete-button-C4rBI4_z.js";var o,s,c,l,u,d,f;e((()=>{n(),i(),o=t(),s={title:`Foundations/ConfirmDeleteButton`,component:a,tags:[`autodocs`],args:{onDelete:()=>alert(`Deleted!`)},parameters:{docs:{description:{component:`Two-click delete button. Click once to arm, click Confirm to execute. Three modes: icon (ghost trash icon), text (Delete button), inline (small footer text).`}}}},c={name:`Icon Mode (default)`,args:{mode:`icon`}},l={name:`Text Mode`,args:{mode:`text`}},u={name:`Inline Mode (card footers)`,args:{mode:`inline`}},d={name:`All Modes Side by Side`,render:()=>(0,o.jsxs)(`div`,{className:`flex items-center gap-6`,children:[(0,o.jsxs)(`div`,{className:`text-center`,children:[(0,o.jsx)(a,{onDelete:()=>{},mode:`icon`}),(0,o.jsx)(`p`,{className:`mt-1 text-[10px] text-muted-foreground`,children:r(`3Y3qJ3`,`icon`)})]}),(0,o.jsxs)(`div`,{className:`text-center`,children:[(0,o.jsx)(a,{onDelete:()=>{},mode:`text`}),(0,o.jsx)(`p`,{className:`mt-1 text-[10px] text-muted-foreground`,children:r(`4ehUu5`,`text`)})]}),(0,o.jsxs)(`div`,{className:`text-center`,children:[(0,o.jsx)(a,{onDelete:()=>{},mode:`inline`}),(0,o.jsx)(`p`,{className:`mt-1 text-[10px] text-muted-foreground`,children:r(`1Ballh`,`inline`)})]})]})},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  name: "Icon Mode (default)",
  args: {
    mode: "icon"
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  name: "Text Mode",
  args: {
    mode: "text"
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  name: "Inline Mode (card footers)",
  args: {
    mode: "inline"
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  name: "All Modes Side by Side",
  render: () => <div className="flex items-center gap-6">
      <div className="text-center">
        <ConfirmDeleteButton onDelete={() => {}} mode="icon" />
        <p className="mt-1 text-[10px] text-muted-foreground">icon</p>
      </div>
      <div className="text-center">
        <ConfirmDeleteButton onDelete={() => {}} mode="text" />
        <p className="mt-1 text-[10px] text-muted-foreground">text</p>
      </div>
      <div className="text-center">
        <ConfirmDeleteButton onDelete={() => {}} mode="inline" />
        <p className="mt-1 text-[10px] text-muted-foreground">inline</p>
      </div>
    </div>
}`,...d.parameters?.docs?.source}}},f=[`IconMode`,`TextMode`,`InlineMode`,`AllModes`]}))();export{d as AllModes,c as IconMode,u as InlineMode,l as TextMode,f as __namedExportsOrder,s as default};