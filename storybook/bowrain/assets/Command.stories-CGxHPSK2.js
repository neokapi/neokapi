import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{t as n}from"./src-Bvgh6dl3.js";import{a as r,c as i,i as a,n as o,o as s,r as c,s as l,t as u}from"./command-B-cloO7Q.js";var d,f,p,m,h;e((()=>{n(),d=t(),f={title:`Foundations/Command`,component:u,tags:[`autodocs`],decorators:[e=>(0,d.jsx)(`div`,{style:{maxWidth:400,padding:16},children:(0,d.jsx)(e,{})})]},p={render:()=>(0,d.jsxs)(u,{className:`rounded-lg border shadow-md`,children:[(0,d.jsx)(a,{placeholder:`Type a command or search...`}),(0,d.jsxs)(s,{children:[(0,d.jsx)(o,{children:`No results found.`}),(0,d.jsxs)(c,{heading:`Actions`,children:[(0,d.jsxs)(r,{children:[(0,d.jsx)(`span`,{children:`New Project`}),(0,d.jsx)(i,{children:`Ctrl+N`})]}),(0,d.jsxs)(r,{children:[(0,d.jsx)(`span`,{children:`Run Flow`}),(0,d.jsx)(i,{children:`Ctrl+R`})]}),(0,d.jsxs)(r,{children:[(0,d.jsx)(`span`,{children:`Push Changes`}),(0,d.jsx)(i,{children:`Ctrl+P`})]})]}),(0,d.jsx)(l,{}),(0,d.jsxs)(c,{heading:`Navigation`,children:[(0,d.jsx)(r,{children:(0,d.jsx)(`span`,{children:`Translation Memory`})}),(0,d.jsx)(r,{children:(0,d.jsx)(`span`,{children:`Terminology`})})]})]})]})},m={render:()=>(0,d.jsxs)(u,{className:`rounded-lg border shadow-md`,children:[(0,d.jsx)(a,{placeholder:`Search...`}),(0,d.jsx)(s,{children:(0,d.jsx)(o,{children:`No results found.`})})]})},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  render: () => <Command className="rounded-lg border shadow-md">
      <CommandInput placeholder="Type a command or search..." />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        <CommandGroup heading="Actions">
          <CommandItem>
            <span>New Project</span>
            <CommandShortcut>Ctrl+N</CommandShortcut>
          </CommandItem>
          <CommandItem>
            <span>Run Flow</span>
            <CommandShortcut>Ctrl+R</CommandShortcut>
          </CommandItem>
          <CommandItem>
            <span>Push Changes</span>
            <CommandShortcut>Ctrl+P</CommandShortcut>
          </CommandItem>
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading="Navigation">
          <CommandItem>
            <span>Translation Memory</span>
          </CommandItem>
          <CommandItem>
            <span>Terminology</span>
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </Command>
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => <Command className="rounded-lg border shadow-md">
      <CommandInput placeholder="Search..." />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
      </CommandList>
    </Command>
}`,...m.parameters?.docs?.source}}},h=[`Default`,`Empty`]}))();export{p as Default,m as Empty,h as __namedExportsOrder,f as default};