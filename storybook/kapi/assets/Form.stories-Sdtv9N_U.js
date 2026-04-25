import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{r as i,t as a}from"./runtime-B587fM_0.js";import{a as o,c as s,i as c,l,n as u,o as d,r as f,s as p,t as m,u as h}from"./form-DsYi2z4r.js";import{n as g,t as _}from"./button-Dd5Xmast.js";import{n as v,t as y}from"./input-CHMoQ1gF.js";var b,x,S,C,w,T,E,D,O,k,A,j,M,N,P,F;t((()=>{i(),b=e(n(),1),h(),v(),g(),x=r(),S={title:`Foundations/Form Primitives`,tags:[`autodocs`],parameters:{docs:{description:{component:`Composable form layout primitives. Use these to build any form UI — from simple config editors to complex schema-driven forms. Follows the shadcn naming convention (FormItem, FormLabel, etc.) without requiring react-hook-form.`}}}},C={name:`FormItem — Basic Field`,render:()=>{let[e,t]=(0,b.useState)(``);return(0,x.jsx)(`div`,{className:`max-w-xs`,children:(0,x.jsxs)(d,{children:[(0,x.jsx)(p,{children:a(`QSDpP`,`Project Name`)}),(0,x.jsx)(u,{children:a(`2t5lIT`,`A short identifier for your project.`)}),(0,x.jsx)(m,{children:(0,x.jsx)(y,{value:e,placeholder:a(`HeRFr`,`my-project`),onChange:e=>t(e.target.value)})})]})})}},w={name:`FormItem — With Validation Error`,render:()=>(0,x.jsx)(`div`,{className:`max-w-xs`,children:(0,x.jsxs)(d,{children:[(0,x.jsx)(p,{children:a(`2Ckqtv`,`Email`)}),(0,x.jsx)(m,{children:(0,x.jsx)(y,{defaultValue:`not-an-email`,className:`border-destructive`})}),(0,x.jsx)(s,{children:a(`Q4H2v`,`Please enter a valid email address.`)})]})})},T={name:`FormItem — Modified from Preset`,render:()=>(0,x.jsxs)(`div`,{className:`max-w-xs space-y-2`,children:[(0,x.jsxs)(d,{modified:!0,children:[(0,x.jsx)(p,{children:a(`4jrQJD`,`Threshold`)}),(0,x.jsx)(u,{children:a(`2av9SH`,`Minimum match score (0–100). Value differs from preset.`)}),(0,x.jsx)(m,{children:(0,x.jsx)(y,{type:`number`,defaultValue:`85`})})]}),(0,x.jsxs)(d,{children:[(0,x.jsx)(p,{children:a(`OseJG`,`Max Results`)}),(0,x.jsx)(u,{children:a(`1Omsvr`,`Unmodified field for comparison.`)}),(0,x.jsx)(m,{children:(0,x.jsx)(y,{type:`number`,defaultValue:`100`})})]})]})},E={name:`FormItem — Disabled`,render:()=>(0,x.jsx)(`div`,{className:`max-w-xs`,children:(0,x.jsxs)(d,{disabled:!0,children:[(0,x.jsx)(p,{disabled:!0,children:a(`1Saj0K`,`Output Path`)}),(0,x.jsx)(u,{children:a(`1D2xH9`,`Disabled when auto-detect is on.`)}),(0,x.jsx)(m,{children:(0,x.jsx)(y,{defaultValue:`/output/report.html`,disabled:!0})})]})})},D={name:`FormItem — With Expandable Help`,render:()=>(0,x.jsx)(`div`,{className:`max-w-xs`,children:(0,x.jsxs)(d,{children:[(0,x.jsx)(p,{children:a(`ytCap`,`Extraction Rules`)}),(0,x.jsx)(m,{children:(0,x.jsx)(y,{defaultValue:`.*`,className:`font-mono`})}),(0,x.jsx)(c,{description:a(`qcx1k`,`Regex patterns that determine which JSON paths are extracted for translation.`),notes:[`Patterns are matched against the full JSON path (e.g., $.messages[*].text).`,`Use .* to extract all string values.`],dependencies:[{property:`extractAll`,condition:`must be false`}]})]})})},O={name:`FormToggle — Boolean Field`,render:()=>{let[e,t]=(0,b.useState)(!0);return(0,x.jsxs)(`div`,{className:`max-w-xs space-y-2`,children:[(0,x.jsx)(l,{checked:e,onCheckedChange:t,label:a(`I6mbY`,`Check Leading Whitespace`),description:a(`3i0pSA`,`Flag text units where leading whitespace differs between source and target.`)}),(0,x.jsx)(l,{checked:!1,onCheckedChange:()=>{},label:a(`1lEUW6`,`Disabled Toggle`),description:a(`1rn5G5`,`This toggle is disabled.`),disabled:!0}),(0,x.jsx)(l,{checked:!0,onCheckedChange:()=>{},label:a(`12g1Bd`,`Modified Toggle`),description:a(`2k7cs6`,`Value differs from active preset.`),modified:!0})]})}},k={name:`FormToggle — Compact Mode`,render:()=>{let[e,t]=(0,b.useState)(!0),[n,r]=(0,b.useState)(!1),[i,o]=(0,b.useState)(!0);return(0,x.jsxs)(`div`,{className:`max-w-xs space-y-0.5`,children:[(0,x.jsx)(l,{checked:e,onCheckedChange:t,label:a(`2AUNg6`,`Leading whitespace`),compact:!0}),(0,x.jsx)(l,{checked:n,onCheckedChange:r,label:a(`2yRl7b`,`Trailing whitespace`),compact:!0}),(0,x.jsx)(l,{checked:i,onCheckedChange:o,label:a(`bWSNH`,`Empty target`),compact:!0})]})}},A={name:`FormInputAction — Path Picker`,render:()=>(0,x.jsx)(`div`,{className:`max-w-sm`,children:(0,x.jsxs)(d,{children:[(0,x.jsx)(p,{children:a(`18RnEW`,`Report File Path`)}),(0,x.jsx)(u,{children:a(`30E3dv`,`Path of the report file to generate.`)}),(0,x.jsxs)(o,{children:[(0,x.jsx)(y,{defaultValue:"${rootDir}/qa-report.html",className:`flex-1 font-mono text-xs h-8`}),(0,x.jsx)(_,{variant:`outline`,size:`sm`,className:`h-8 text-xs shrink-0`,children:a(`471cyz`,`Browse`)})]})]})})},j={name:`FormInputAction — Multiple Actions`,render:()=>(0,x.jsx)(`div`,{className:`max-w-sm`,children:(0,x.jsxs)(d,{children:[(0,x.jsx)(p,{children:a(`4FipZK`,`TM File`)}),(0,x.jsxs)(o,{children:[(0,x.jsx)(y,{defaultValue:`/data/project.tmx`,className:`flex-1 font-mono text-xs h-8`}),(0,x.jsx)(_,{variant:`outline`,size:`sm`,className:`h-8 text-xs shrink-0`,children:a(`471cyz`,`Browse`)}),(0,x.jsx)(_,{variant:`ghost`,size:`sm`,className:`h-8 text-xs shrink-0`,children:a(`4jW1DJ`,`Clear`)})]})]})})},M={name:`FormFieldGroup — Non-Collapsible`,render:()=>(0,x.jsx)(`div`,{className:`max-w-xs`,children:(0,x.jsx)(f,{label:a(`1HGBFg`,`General Settings`),children:(0,x.jsxs)(`div`,{className:`space-y-2`,children:[(0,x.jsxs)(d,{children:[(0,x.jsx)(p,{children:a(`zOtmS`,`Name`)}),(0,x.jsx)(m,{children:(0,x.jsx)(y,{defaultValue:`My Project`})})]}),(0,x.jsxs)(d,{children:[(0,x.jsx)(p,{children:a(`3jjyXb`,`Description`)}),(0,x.jsx)(m,{children:(0,x.jsx)(y,{placeholder:a(`1SSxpW`,`Optional description...`)})})]})]})})})},N={name:`FormFieldGroup — Collapsible`,render:()=>(0,x.jsxs)(`div`,{className:`max-w-xs space-y-4`,children:[(0,x.jsx)(f,{label:a(`2UVgx8`,`Whitespace Checks`),collapsible:!0,children:(0,x.jsxs)(`div`,{className:`space-y-1`,children:[(0,x.jsx)(l,{checked:!0,onCheckedChange:()=>{},label:a(`13kQVh`,`Leading whitespace`),compact:!0}),(0,x.jsx)(l,{checked:!0,onCheckedChange:()=>{},label:a(`1UmRur`,`Trailing whitespace`),compact:!0}),(0,x.jsx)(l,{checked:!1,onCheckedChange:()=>{},label:a(`4iW97K`,`Double spaces`),compact:!0})]})}),(0,x.jsx)(f,{label:a(`4aJaeY`,`Content Checks`),collapsible:!0,defaultCollapsed:!0,children:(0,x.jsxs)(`div`,{className:`space-y-1`,children:[(0,x.jsx)(l,{checked:!0,onCheckedChange:()=>{},label:a(`TdKmV`,`Empty target`),compact:!0}),(0,x.jsx)(l,{checked:!0,onCheckedChange:()=>{},label:a(`5XNlx`,`Target same as source`),compact:!0})]})}),(0,x.jsx)(f,{label:a(`1TWIlR`,`Output`),collapsible:!0,defaultCollapsed:!0,children:(0,x.jsx)(`div`,{className:`space-y-2`,children:(0,x.jsxs)(d,{children:[(0,x.jsx)(p,{children:a(`1eYyLD`,`Report Path`)}),(0,x.jsxs)(o,{children:[(0,x.jsx)(y,{defaultValue:"${rootDir}/qa-report.html",className:`flex-1 font-mono text-xs h-8`}),(0,x.jsx)(_,{variant:`outline`,size:`sm`,className:`h-8 text-xs shrink-0`,children:a(`PxVl9`,`Browse`)})]})]})})})]})},P={name:`Composed — Mini Config Editor`,render:()=>{let[e,t]=(0,b.useState)(!1),[n,r]=(0,b.useState)(`95`),[i,s]=(0,b.useState)(`html`);return(0,x.jsxs)(`div`,{className:`max-w-sm space-y-4`,children:[(0,x.jsx)(f,{label:a(`2SxT7Q`,`Translation Memory`),children:(0,x.jsxs)(`div`,{className:`space-y-2`,children:[(0,x.jsx)(l,{checked:e,onCheckedChange:t,label:a(`1ySJio`,`Use Translation Memory`),description:a(`2iiKBn`,`Leverage existing translations for pre-population.`)}),(0,x.jsxs)(d,{disabled:!e,children:[(0,x.jsx)(p,{disabled:!e,children:a(`GlQtL`,`TM File`)}),(0,x.jsxs)(o,{children:[(0,x.jsx)(y,{defaultValue:`/data/project.tmx`,disabled:!e,className:`flex-1 font-mono text-xs h-8`}),(0,x.jsx)(_,{variant:`outline`,size:`sm`,disabled:!e,className:`h-8 text-xs shrink-0`,children:a(`PxVl9`,`Browse`)})]})]}),(0,x.jsxs)(d,{disabled:!e,children:[(0,x.jsx)(p,{disabled:!e,children:a(`3ZGQe4`,`Match Threshold`)}),(0,x.jsx)(u,{children:a(`1LLT7B`,`Minimum similarity score (0–100).`)}),(0,x.jsx)(m,{children:(0,x.jsx)(y,{type:`number`,min:0,max:100,value:n,disabled:!e,className:`h-8 text-xs`,onChange:e=>r(e.target.value)})})]})]})}),(0,x.jsx)(f,{label:a(`1TWIlR`,`Output`),collapsible:!0,children:(0,x.jsx)(`div`,{className:`space-y-2`,children:(0,x.jsxs)(d,{children:[(0,x.jsx)(p,{children:a(`3N0pzA`,`Report Format`)}),(0,x.jsx)(m,{children:a(`2RAW9o`,`{format, select, }`,{format:i})})]})})})]})}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  name: "FormItem — Basic Field",
  render: () => {
    const [value, setValue] = useState("");
    return <div className="max-w-xs">
        <FormItem>
          <FormLabel>Project Name</FormLabel>
          <FormDescription>A short identifier for your project.</FormDescription>
          <FormControl>
            <Input value={value} placeholder="my-project" onChange={(e: React.ChangeEvent<HTMLInputElement>) => setValue(e.target.value)} />
          </FormControl>
        </FormItem>
      </div>;
  }
}`,...C.parameters?.docs?.source}}},w.parameters={...w.parameters,docs:{...w.parameters?.docs,source:{originalSource:`{
  name: "FormItem — With Validation Error",
  render: () => <div className="max-w-xs">
      <FormItem>
        <FormLabel>Email</FormLabel>
        <FormControl>
          <Input defaultValue="not-an-email" className="border-destructive" />
        </FormControl>
        <FormMessage>Please enter a valid email address.</FormMessage>
      </FormItem>
    </div>
}`,...w.parameters?.docs?.source}}},T.parameters={...T.parameters,docs:{...T.parameters?.docs,source:{originalSource:`{
  name: "FormItem — Modified from Preset",
  render: () => <div className="max-w-xs space-y-2">
      <FormItem modified>
        <FormLabel>Threshold</FormLabel>
        <FormDescription>Minimum match score (0–100). Value differs from preset.</FormDescription>
        <FormControl>
          <Input type="number" defaultValue="85" />
        </FormControl>
      </FormItem>
      <FormItem>
        <FormLabel>Max Results</FormLabel>
        <FormDescription>Unmodified field for comparison.</FormDescription>
        <FormControl>
          <Input type="number" defaultValue="100" />
        </FormControl>
      </FormItem>
    </div>
}`,...T.parameters?.docs?.source}}},E.parameters={...E.parameters,docs:{...E.parameters?.docs,source:{originalSource:`{
  name: "FormItem — Disabled",
  render: () => <div className="max-w-xs">
      <FormItem disabled>
        <FormLabel disabled>Output Path</FormLabel>
        <FormDescription>Disabled when auto-detect is on.</FormDescription>
        <FormControl>
          <Input defaultValue="/output/report.html" disabled />
        </FormControl>
      </FormItem>
    </div>
}`,...E.parameters?.docs?.source}}},D.parameters={...D.parameters,docs:{...D.parameters?.docs,source:{originalSource:`{
  name: "FormItem — With Expandable Help",
  render: () => <div className="max-w-xs">
      <FormItem>
        <FormLabel>Extraction Rules</FormLabel>
        <FormControl>
          <Input defaultValue=".*" className="font-mono" />
        </FormControl>
        <FormHelpText description="Regex patterns that determine which JSON paths are extracted for translation." notes={["Patterns are matched against the full JSON path (e.g., $.messages[*].text).", "Use .* to extract all string values."]} dependencies={[{
        property: "extractAll",
        condition: "must be false"
      }]} />
      </FormItem>
    </div>
}`,...D.parameters?.docs?.source}}},O.parameters={...O.parameters,docs:{...O.parameters?.docs,source:{originalSource:`{
  name: "FormToggle — Boolean Field",
  render: () => {
    const [checked, setChecked] = useState(true);
    return <div className="max-w-xs space-y-2">
        <FormToggle checked={checked} onCheckedChange={setChecked} label="Check Leading Whitespace" description="Flag text units where leading whitespace differs between source and target." />
        <FormToggle checked={false} onCheckedChange={() => {}} label="Disabled Toggle" description="This toggle is disabled." disabled />
        <FormToggle checked={true} onCheckedChange={() => {}} label="Modified Toggle" description="Value differs from active preset." modified />
      </div>;
  }
}`,...O.parameters?.docs?.source}}},k.parameters={...k.parameters,docs:{...k.parameters?.docs,source:{originalSource:`{
  name: "FormToggle — Compact Mode",
  render: () => {
    const [a, setA] = useState(true);
    const [b, setB] = useState(false);
    const [c, setC] = useState(true);
    return <div className="max-w-xs space-y-0.5">
        <FormToggle checked={a} onCheckedChange={setA} label="Leading whitespace" compact />
        <FormToggle checked={b} onCheckedChange={setB} label="Trailing whitespace" compact />
        <FormToggle checked={c} onCheckedChange={setC} label="Empty target" compact />
      </div>;
  }
}`,...k.parameters?.docs?.source}}},A.parameters={...A.parameters,docs:{...A.parameters?.docs,source:{originalSource:`{
  name: "FormInputAction — Path Picker",
  render: () => <div className="max-w-sm">
      <FormItem>
        <FormLabel>Report File Path</FormLabel>
        <FormDescription>Path of the report file to generate.</FormDescription>
        <FormInputAction>
          <Input defaultValue="\${rootDir}/qa-report.html" className="flex-1 font-mono text-xs h-8" />
          <Button variant="outline" size="sm" className="h-8 text-xs shrink-0">
            Browse
          </Button>
        </FormInputAction>
      </FormItem>
    </div>
}`,...A.parameters?.docs?.source}}},j.parameters={...j.parameters,docs:{...j.parameters?.docs,source:{originalSource:`{
  name: "FormInputAction — Multiple Actions",
  render: () => <div className="max-w-sm">
      <FormItem>
        <FormLabel>TM File</FormLabel>
        <FormInputAction>
          <Input defaultValue="/data/project.tmx" className="flex-1 font-mono text-xs h-8" />
          <Button variant="outline" size="sm" className="h-8 text-xs shrink-0">
            Browse
          </Button>
          <Button variant="ghost" size="sm" className="h-8 text-xs shrink-0">
            Clear
          </Button>
        </FormInputAction>
      </FormItem>
    </div>
}`,...j.parameters?.docs?.source}}},M.parameters={...M.parameters,docs:{...M.parameters?.docs,source:{originalSource:`{
  name: "FormFieldGroup — Non-Collapsible",
  render: () => <div className="max-w-xs">
      <FormFieldGroup label="General Settings">
        <div className="space-y-2">
          <FormItem>
            <FormLabel>Name</FormLabel>
            <FormControl>
              <Input defaultValue="My Project" />
            </FormControl>
          </FormItem>
          <FormItem>
            <FormLabel>Description</FormLabel>
            <FormControl>
              <Input placeholder="Optional description..." />
            </FormControl>
          </FormItem>
        </div>
      </FormFieldGroup>
    </div>
}`,...M.parameters?.docs?.source}}},N.parameters={...N.parameters,docs:{...N.parameters?.docs,source:{originalSource:`{
  name: "FormFieldGroup — Collapsible",
  render: () => <div className="max-w-xs space-y-4">
      <FormFieldGroup label="Whitespace Checks" collapsible>
        <div className="space-y-1">
          <FormToggle checked={true} onCheckedChange={() => {}} label="Leading whitespace" compact />
          <FormToggle checked={true} onCheckedChange={() => {}} label="Trailing whitespace" compact />
          <FormToggle checked={false} onCheckedChange={() => {}} label="Double spaces" compact />
        </div>
      </FormFieldGroup>

      <FormFieldGroup label="Content Checks" collapsible defaultCollapsed>
        <div className="space-y-1">
          <FormToggle checked={true} onCheckedChange={() => {}} label="Empty target" compact />
          <FormToggle checked={true} onCheckedChange={() => {}} label="Target same as source" compact />
        </div>
      </FormFieldGroup>

      <FormFieldGroup label="Output" collapsible defaultCollapsed>
        <div className="space-y-2">
          <FormItem>
            <FormLabel>Report Path</FormLabel>
            <FormInputAction>
              <Input defaultValue="\${rootDir}/qa-report.html" className="flex-1 font-mono text-xs h-8" />
              <Button variant="outline" size="sm" className="h-8 text-xs shrink-0">
                Browse
              </Button>
            </FormInputAction>
          </FormItem>
        </div>
      </FormFieldGroup>
    </div>
}`,...N.parameters?.docs?.source}}},P.parameters={...P.parameters,docs:{...P.parameters?.docs,source:{originalSource:`{
  name: "Composed — Mini Config Editor",
  render: () => {
    const [useTM, setUseTM] = useState(false);
    const [threshold, setThreshold] = useState("95");
    const [format, setFormat] = useState("html");
    return <div className="max-w-sm space-y-4">
        <FormFieldGroup label="Translation Memory">
          <div className="space-y-2">
            <FormToggle checked={useTM} onCheckedChange={setUseTM} label="Use Translation Memory" description="Leverage existing translations for pre-population." />

            <FormItem disabled={!useTM}>
              <FormLabel disabled={!useTM}>TM File</FormLabel>
              <FormInputAction>
                <Input defaultValue="/data/project.tmx" disabled={!useTM} className="flex-1 font-mono text-xs h-8" />
                <Button variant="outline" size="sm" disabled={!useTM} className="h-8 text-xs shrink-0">
                  Browse
                </Button>
              </FormInputAction>
            </FormItem>

            <FormItem disabled={!useTM}>
              <FormLabel disabled={!useTM}>Match Threshold</FormLabel>
              <FormDescription>Minimum similarity score (0–100).</FormDescription>
              <FormControl>
                <Input type="number" min={0} max={100} value={threshold} disabled={!useTM} className="h-8 text-xs" onChange={(e: React.ChangeEvent<HTMLInputElement>) => setThreshold(e.target.value)} />
              </FormControl>
            </FormItem>
          </div>
        </FormFieldGroup>

        <FormFieldGroup label="Output" collapsible>
          <div className="space-y-2">
            <FormItem>
              <FormLabel>Report Format</FormLabel>
              <FormControl>
                <Select value={format} onValueChange={setFormat}>
                  <SelectTrigger className="h-8 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="html">HTML file</SelectItem>
                    <SelectItem value="tsv">Tab-delimited file</SelectItem>
                    <SelectItem value="xml">XML file</SelectItem>
                  </SelectContent>
                </Select>
              </FormControl>
            </FormItem>
          </div>
        </FormFieldGroup>
      </div>;
  }
}`,...P.parameters?.docs?.source}}},F=[`BasicField`,`FieldWithError`,`FieldWithModifiedIndicator`,`DisabledField`,`FieldWithHelp`,`Toggle`,`ToggleCompact`,`InputWithAction`,`InputWithMultipleActions`,`FieldGroup`,`CollapsibleGroup`,`ComposedConfigEditor`]}))();export{C as BasicField,N as CollapsibleGroup,P as ComposedConfigEditor,E as DisabledField,M as FieldGroup,w as FieldWithError,D as FieldWithHelp,T as FieldWithModifiedIndicator,A as InputWithAction,j as InputWithMultipleActions,O as Toggle,k as ToggleCompact,F as __namedExportsOrder,S as default};