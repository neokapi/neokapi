import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./DocumentPreview-CTwMI6vl.js";import{M as i,l as a}from"./fixtures-D8OXoGLB.js";import{n as o,r as s}from"./decorators-BptasVUO.js";var c,l,u,d,f,p,m,h,g,_;e((()=>{n(),a(),o(),c=t(),{fn:l}=__STORYBOOK_MODULE_TEST__,u={title:`Editor/Formatting/DocumentPreview`,component:r,tags:[`autodocs`],decorators:[s,e=>(0,c.jsx)(`div`,{style:{width:`100%`,height:500},children:(0,c.jsx)(e,{})})],args:{projectId:`proj-demo-1`,itemName:`messages.json`,targetLocale:`fr-FR`,onBlockSelect:l(),blocks:i}},d={},f={args:{selectedBlockId:`blk-2`}},p={args:{previewContentMode:`source`,selectedBlockId:`blk-1`}},m={args:{previewContentMode:`target`,selectedBlockId:`blk-1`}},h={args:{previewContentMode:`pseudo`,selectedBlockId:`blk-1`}},g={args:{selectedBlockId:`blk-2`,spacerHeight:400,onSpacerPosition:l(),onContentHeight:l()},decorators:[s,e=>(0,c.jsx)(`div`,{style:{width:`100%`,height:`auto`,minHeight:600},children:(0,c.jsx)(e,{})})]},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{}`,...d.parameters?.docs?.source},description:{story:`Default preview — source content mode, no block selected`,...d.parameters?.docs?.description}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  args: {
    selectedBlockId: "blk-2"
  }
}`,...f.parameters?.docs?.source},description:{story:`Preview with a selected block highlighted`,...f.parameters?.docs?.description}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  args: {
    previewContentMode: "source",
    selectedBlockId: "blk-1"
  }
}`,...p.parameters?.docs?.source},description:{story:`Controlled source mode via prop`,...p.parameters?.docs?.description}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    previewContentMode: "target",
    selectedBlockId: "blk-1"
  }
}`,...m.parameters?.docs?.source},description:{story:`Controlled target mode via prop`,...m.parameters?.docs?.description}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    previewContentMode: "pseudo",
    selectedBlockId: "blk-1"
  }
}`,...h.parameters?.docs?.source},description:{story:`Pseudo mode — source text rendered with accent mapping, no server action needed`,...h.parameters?.docs?.description}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    selectedBlockId: "blk-2",
    spacerHeight: 400,
    onSpacerPosition: fn(),
    onContentHeight: fn()
  },
  decorators: [withProviders, Story => <div style={{
    width: "100%",
    height: "auto",
    minHeight: 600
  }}>
        <Story />
      </div>]
}`,...g.parameters?.docs?.source},description:{story:`Inline mode with spacer height — used inside VisualEditorLayout`,...g.parameters?.docs?.description}}},_=[`Default`,`WithSelectedBlock`,`SourceMode`,`TargetMode`,`PseudoMode`,`InlineMode`]}))();export{d as Default,g as InlineMode,h as PseudoMode,p as SourceMode,m as TargetMode,f as WithSelectedBlock,_ as __namedExportsOrder,u as default};