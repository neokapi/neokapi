import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{a as n,c as r,i,n as a,o,r as s,s as c}from"./tool-fallback-BpC4r97z.js";var l,u,d,f,p,m,h,g,_;e((()=>{r(),l=t(),u={title:`Bravo/Assistant UI/Tool Fallback`,tags:[`autodocs`],parameters:{layout:`centered`,docs:{description:{component:`ToolFallback uses compound components (Root, Trigger, Content, Args, Result, Error)
that can be rendered standalone without the assistant-ui runtime.`}}},decorators:[e=>(0,l.jsx)(`div`,{className:`w-[500px] bg-background text-foreground p-4`,children:(0,l.jsx)(e,{})})]},d={render:()=>(0,l.jsxs)(o,{defaultOpen:!0,children:[(0,l.jsx)(c,{toolName:`run_flow`,status:{type:`complete`}}),(0,l.jsxs)(s,{children:[(0,l.jsx)(a,{argsText:JSON.stringify({flow:`pseudo-translate`,target_lang:`qps`},null,2)}),(0,l.jsx)(n,{result:{blocks_processed:42,blocks_skipped:3}})]})]})},f={render:()=>(0,l.jsxs)(o,{children:[(0,l.jsx)(c,{toolName:`connector_push`,status:{type:`running`}}),(0,l.jsx)(s,{children:(0,l.jsx)(a,{argsText:JSON.stringify({connector_id:`git-main`,project_id:`proj-1`},null,2)})})]})},p={render:()=>(0,l.jsxs)(o,{defaultOpen:!0,children:[(0,l.jsx)(c,{toolName:`connector_push`,status:{type:`requires-action`,reason:`interrupt`}}),(0,l.jsx)(s,{children:(0,l.jsx)(a,{argsText:JSON.stringify({connector_id:`git-main`,project_id:`proj-1`},null,2)})})]})},m={render:()=>(0,l.jsxs)(o,{defaultOpen:!0,className:`border-muted-foreground/30 bg-muted/30`,children:[(0,l.jsx)(c,{toolName:`run_flow`,status:{type:`incomplete`,reason:`cancelled`}}),(0,l.jsxs)(s,{children:[(0,l.jsx)(i,{status:{type:`incomplete`,reason:`cancelled`,error:`User cancelled the operation`}}),(0,l.jsx)(a,{argsText:JSON.stringify({flow:`pseudo-translate`},null,2),className:`opacity-60`})]})]})},h={render:()=>(0,l.jsxs)(o,{defaultOpen:!0,children:[(0,l.jsx)(c,{toolName:`run_flow`,status:{type:`incomplete`,reason:`error`}}),(0,l.jsxs)(s,{children:[(0,l.jsx)(i,{status:{type:`incomplete`,reason:`error`,error:`Flow execution timed out after 30s`}}),(0,l.jsx)(a,{argsText:JSON.stringify({flow:`translate`,provider:`openai`},null,2)})]})]})},g={render:()=>(0,l.jsxs)(o,{children:[(0,l.jsx)(c,{toolName:`run_flow`,status:{type:`complete`}}),(0,l.jsxs)(s,{children:[(0,l.jsx)(a,{argsText:JSON.stringify({flow:`pseudo-translate`},null,2)}),(0,l.jsx)(n,{result:{blocks_processed:10}})]})]})},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  render: () => <ToolFallbackRoot defaultOpen>
      <ToolFallbackTrigger toolName="run_flow" status={{
      type: "complete"
    }} />
      <ToolFallbackContent>
        <ToolFallbackArgs argsText={JSON.stringify({
        flow: "pseudo-translate",
        target_lang: "qps"
      }, null, 2)} />
        <ToolFallbackResult result={{
        blocks_processed: 42,
        blocks_skipped: 3
      }} />
      </ToolFallbackContent>
    </ToolFallbackRoot>
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: () => <ToolFallbackRoot>
      <ToolFallbackTrigger toolName="connector_push" status={{
      type: "running"
    }} />
      <ToolFallbackContent>
        <ToolFallbackArgs argsText={JSON.stringify({
        connector_id: "git-main",
        project_id: "proj-1"
      }, null, 2)} />
      </ToolFallbackContent>
    </ToolFallbackRoot>
}`,...f.parameters?.docs?.source}}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  render: () => <ToolFallbackRoot defaultOpen>
      <ToolFallbackTrigger toolName="connector_push" status={{
      type: "requires-action",
      reason: "interrupt"
    }} />
      <ToolFallbackContent>
        <ToolFallbackArgs argsText={JSON.stringify({
        connector_id: "git-main",
        project_id: "proj-1"
      }, null, 2)} />
      </ToolFallbackContent>
    </ToolFallbackRoot>
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => <ToolFallbackRoot defaultOpen className="border-muted-foreground/30 bg-muted/30">
      <ToolFallbackTrigger toolName="run_flow" status={{
      type: "incomplete",
      reason: "cancelled"
    }} />
      <ToolFallbackContent>
        <ToolFallbackError status={{
        type: "incomplete",
        reason: "cancelled",
        error: "User cancelled the operation"
      }} />
        <ToolFallbackArgs argsText={JSON.stringify({
        flow: "pseudo-translate"
      }, null, 2)} className="opacity-60" />
      </ToolFallbackContent>
    </ToolFallbackRoot>
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  render: () => <ToolFallbackRoot defaultOpen>
      <ToolFallbackTrigger toolName="run_flow" status={{
      type: "incomplete",
      reason: "error"
    }} />
      <ToolFallbackContent>
        <ToolFallbackError status={{
        type: "incomplete",
        reason: "error",
        error: "Flow execution timed out after 30s"
      }} />
        <ToolFallbackArgs argsText={JSON.stringify({
        flow: "translate",
        provider: "openai"
      }, null, 2)} />
      </ToolFallbackContent>
    </ToolFallbackRoot>
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  render: () => <ToolFallbackRoot>
      <ToolFallbackTrigger toolName="run_flow" status={{
      type: "complete"
    }} />
      <ToolFallbackContent>
        <ToolFallbackArgs argsText={JSON.stringify({
        flow: "pseudo-translate"
      }, null, 2)} />
        <ToolFallbackResult result={{
        blocks_processed: 10
      }} />
      </ToolFallbackContent>
    </ToolFallbackRoot>
}`,...g.parameters?.docs?.source}}},_=[`Complete`,`Running`,`RequiresAction`,`Cancelled`,`WithError`,`Collapsed`]}))();export{m as Cancelled,g as Collapsed,d as Complete,p as RequiresAction,f as Running,h as WithError,_ as __namedExportsOrder,u as default};