import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./confirm-delete-button-Byh0BHlp.js";var i,a,o,s,c,l,u;e((()=>{n(),i=t(),a={title:`Foundations/ConfirmDeleteButton`,component:r,tags:[`autodocs`],args:{onDelete:()=>alert(`Deleted!`)},parameters:{docs:{description:{component:`Two-click delete button. Click once to arm, click Confirm to execute. Three modes: icon (ghost trash icon), text (Delete button), inline (small footer text).`}}}},o={name:`Icon Mode (default)`,args:{mode:`icon`}},s={name:`Text Mode`,args:{mode:`text`}},c={name:`Inline Mode (card footers)`,args:{mode:`inline`}},l={name:`All Modes Side by Side`,render:()=>(0,i.jsxs)(`div`,{className:`flex items-center gap-6`,children:[(0,i.jsxs)(`div`,{className:`text-center`,children:[(0,i.jsx)(r,{onDelete:()=>{},mode:`icon`}),(0,i.jsx)(`p`,{className:`mt-1 text-[10px] text-muted-foreground`,children:`icon`})]}),(0,i.jsxs)(`div`,{className:`text-center`,children:[(0,i.jsx)(r,{onDelete:()=>{},mode:`text`}),(0,i.jsx)(`p`,{className:`mt-1 text-[10px] text-muted-foreground`,children:`text`})]}),(0,i.jsxs)(`div`,{className:`text-center`,children:[(0,i.jsx)(r,{onDelete:()=>{},mode:`inline`}),(0,i.jsx)(`p`,{className:`mt-1 text-[10px] text-muted-foreground`,children:`inline`})]})]})},o.parameters={...o.parameters,docs:{...o.parameters?.docs,source:{originalSource:`{
  name: "Icon Mode (default)",
  args: {
    mode: "icon"
  }
}`,...o.parameters?.docs?.source}}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  name: "Text Mode",
  args: {
    mode: "text"
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  name: "Inline Mode (card footers)",
  args: {
    mode: "inline"
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
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
}`,...l.parameters?.docs?.source}}},u=[`IconMode`,`TextMode`,`InlineMode`,`AllModes`]}))();export{l as AllModes,o as IconMode,c as InlineMode,s as TextMode,u as __namedExportsOrder,a as default};