import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{J as n,t as r}from"./dist-M2wk2DTe.js";import{U as i,W as a}from"./iframe-Cr-Gz2Kj.js";import{a as o,i as s,n as c,r as l,t as u}from"./fixtures-aui-C00XD-8s.js";import{r as d,t as f}from"./fixtures-DC2kBE5i.js";function p({messages:e=[],isRunning:t=!1,children:r}){return(0,m.jsx)(m.Fragment,{children:r(n({messages:e,isRunning:t,convertMessage:e=>e,onNew:async()=>{},onCancel:async()=>{}}))})}var m,h,g,_,v,y,b,x,S,C;e((()=>{r(),a(),f(),u(),m=t(),{fn:h}=__STORYBOOK_MODULE_TEST__,g={title:`Bravo/BravoSidebar`,component:i,tags:[`autodocs`],parameters:{layout:`fullscreen`}},_={render:()=>(0,m.jsx)(p,{children:e=>(0,m.jsx)(i,{open:!0,onOpenChange:h(),runtime:e,view:`list`,onBack:h(),conversationListProps:{conversations:d,onSelect:h(),onDelete:h(),onNew:h()}})})},v={render:()=>(0,m.jsx)(p,{messages:l,children:e=>(0,m.jsx)(i,{open:!0,onOpenChange:h(),runtime:e,view:`chat`,onBack:h(),activeTitle:`Translate French files`,mode:`ask`,onModeChange:h(),onShowConfig:h()})})},y={render:()=>(0,m.jsx)(p,{messages:s,isRunning:!0,children:e=>(0,m.jsx)(i,{open:!0,onOpenChange:h(),runtime:e,view:`chat`,onBack:h(),activeTitle:`Translate French files`,mode:`coworker`,onModeChange:h()})})},b={render:()=>(0,m.jsx)(p,{messages:[o,c],children:e=>(0,m.jsx)(i,{open:!0,onOpenChange:h(),runtime:e,view:`chat`,onBack:h(),activeTitle:`Project analysis`,mode:`ask`,onModeChange:h()})})},x={render:()=>(0,m.jsx)(p,{messages:[o],children:e=>(0,m.jsx)(i,{open:!0,onOpenChange:h(),runtime:e,view:`chat`,onBack:h(),activeTitle:`New conversation`,coldStarting:!0})})},S={render:()=>(0,m.jsx)(p,{children:e=>(0,m.jsx)(i,{open:!0,onOpenChange:h(),runtime:e,view:`chat`,onBack:h(),mode:`ask`,onModeChange:h()})})},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  render: () => <MockRuntimeWrapper>
      {runtime => <BravoSidebar open onOpenChange={fn()} runtime={runtime} view="list" onBack={fn()} conversationListProps={{
      conversations: sampleConversations,
      onSelect: fn(),
      onDelete: fn(),
      onNew: fn()
    }} />}
    </MockRuntimeWrapper>
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  render: () => <MockRuntimeWrapper messages={sampleAuiMessages}>
      {runtime => <BravoSidebar open onOpenChange={fn()} runtime={runtime} view="chat" onBack={fn()} activeTitle="Translate French files" mode="ask" onModeChange={fn()} onShowConfig={fn()} />}
    </MockRuntimeWrapper>
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  render: () => <MockRuntimeWrapper messages={sampleAuiStreamingMessages} isRunning>
      {runtime => <BravoSidebar open onOpenChange={fn()} runtime={runtime} view="chat" onBack={fn()} activeTitle="Translate French files" mode="coworker" onModeChange={fn()} />}
    </MockRuntimeWrapper>
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  render: () => <MockRuntimeWrapper messages={[sampleAuiUserMessage, sampleAuiMarkdownMessage]}>
      {runtime => <BravoSidebar open onOpenChange={fn()} runtime={runtime} view="chat" onBack={fn()} activeTitle="Project analysis" mode="ask" onModeChange={fn()} />}
    </MockRuntimeWrapper>
}`,...b.parameters?.docs?.source}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  render: () => <MockRuntimeWrapper messages={[sampleAuiUserMessage]}>
      {runtime => <BravoSidebar open onOpenChange={fn()} runtime={runtime} view="chat" onBack={fn()} activeTitle="New conversation" coldStarting />}
    </MockRuntimeWrapper>
}`,...x.parameters?.docs?.source}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  render: () => <MockRuntimeWrapper>
      {runtime => <BravoSidebar open onOpenChange={fn()} runtime={runtime} view="chat" onBack={fn()} mode="ask" onModeChange={fn()} />}
    </MockRuntimeWrapper>
}`,...S.parameters?.docs?.source}}},C=[`ConversationList`,`ActiveChat`,`Streaming`,`RichMarkdown`,`ColdStart`,`Empty`]}))();export{v as ActiveChat,x as ColdStart,_ as ConversationList,S as Empty,b as RichMarkdown,y as Streaming,C as __namedExportsOrder,g as default};