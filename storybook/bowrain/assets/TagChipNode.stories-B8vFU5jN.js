import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{t as n}from"./src-Q72DuJW2.js";import{o as r}from"./CodedTextDisplay-DKJcc4pM.js";import{c as i,l as a,n as o,o as s,p as c,y as l}from"./fixtures-D8OXoGLB.js";var u,d,f,p,m,h,g;e((()=>{n(),a(),u=t(),d={title:`Editor/Tags/TagChipNode`,component:r,tags:[`autodocs`],decorators:[e=>(0,u.jsx)(`div`,{style:{padding:16},children:(0,u.jsx)(e,{})})],parameters:{docs:{description:{component:"TagChipNode is a Lexical DecoratorNode that renders a TagChipComponent.\nSince Lexical nodes require an editor context, these stories render the\nvisual output directly via TagChipComponent with the `locked` prop that\nTagChipNode passes through its `decorate()` method."}}}},f={args:{spanInfo:o,locked:!0}},p={args:{spanInfo:l,locked:!1}},m={args:{spanInfo:c,locked:!0}},h={render:()=>(0,u.jsxs)(`div`,{style:{display:`flex`,flexWrap:`wrap`,gap:8,alignItems:`center`},children:[(0,u.jsx)(r,{spanInfo:o,locked:!0}),(0,u.jsx)(r,{spanInfo:o,locked:!1}),(0,u.jsx)(r,{spanInfo:l,locked:!0}),(0,u.jsx)(r,{spanInfo:l,locked:!1}),(0,u.jsx)(r,{spanInfo:s,locked:!0}),(0,u.jsx)(r,{spanInfo:s,locked:!1}),(0,u.jsx)(r,{spanInfo:c,locked:!0}),(0,u.jsx)(r,{spanInfo:i,locked:!0})]})},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    spanInfo: boldOpen,
    locked: true
  }
}`,...f.parameters?.docs?.source},description:{story:`A locked tag — TagChipNode sets locked=true when the span is non-deletable.`,...f.parameters?.docs?.description}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    spanInfo: linkOpen,
    locked: false
  }
}`,...p.parameters?.docs?.source},description:{story:`An unlocked tag — deletable spans render without the dashed border.`,...p.parameters?.docs?.description}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    spanInfo: lineBreak,
    locked: true
  }
}`,...m.parameters?.docs?.source},description:{story:`Placeholder tags (self-closing) are always rendered as locked by TagChipNode.`,...m.parameters?.docs?.description}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    display: "flex",
    flexWrap: "wrap",
    gap: 8,
    alignItems: "center"
  }}>
      <TagChipComponent spanInfo={boldOpen} locked />
      <TagChipComponent spanInfo={boldOpen} locked={false} />
      <TagChipComponent spanInfo={linkOpen} locked />
      <TagChipComponent spanInfo={linkOpen} locked={false} />
      <TagChipComponent spanInfo={codeOpen} locked />
      <TagChipComponent spanInfo={codeOpen} locked={false} />
      <TagChipComponent spanInfo={lineBreak} locked />
      <TagChipComponent spanInfo={imgTag} locked />
    </div>
}`,...h.parameters?.docs?.source},description:{story:`All node variants side by side showing locked vs unlocked rendering.`,...h.parameters?.docs?.description}}},g=[`LockedBold`,`UnlockedLink`,`LockedLineBreak`,`AllVariants`]}))();export{h as AllVariants,f as LockedBold,m as LockedLineBreak,p as UnlockedLink,g as __namedExportsOrder,d as default};