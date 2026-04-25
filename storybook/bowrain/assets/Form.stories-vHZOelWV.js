import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{a as i,c as a,i as o,l as s,n as c,o as l,r as u,s as d,t as f,u as p}from"./form-DM53yz-1.js";import{n as m,t as h}from"./button-MBstDm8k.js";import{n as g,t as _}from"./input-C1MWkU_r.js";import{c as v,i as y,n as b,o as x,s as S,t as C}from"./select-BYYGJvgy.js";var w,T,E,D,O,k,A,j,M,N,P,F,I,L,R,z;t((()=>{w=e(n(),1),p(),g(),m(),v(),T=r(),E={title:`Foundations/Form Primitives`,tags:[`autodocs`],parameters:{docs:{description:{component:`Composable form layout primitives. Use these to build any form UI — from simple config editors to complex schema-driven forms. Follows the shadcn naming convention (FormItem, FormLabel, etc.) without requiring react-hook-form.`}}}},D={name:`FormItem — Basic Field`,render:()=>{let[e,t]=(0,w.useState)(``);return(0,T.jsx)(`div`,{className:`max-w-xs`,children:(0,T.jsxs)(l,{children:[(0,T.jsx)(d,{children:`Project Name`}),(0,T.jsx)(c,{children:`A short identifier for your project.`}),(0,T.jsx)(f,{children:(0,T.jsx)(_,{value:e,placeholder:`my-project`,onChange:e=>t(e.target.value)})})]})})}},O={name:`FormItem — With Validation Error`,render:()=>(0,T.jsx)(`div`,{className:`max-w-xs`,children:(0,T.jsxs)(l,{children:[(0,T.jsx)(d,{children:`Email`}),(0,T.jsx)(f,{children:(0,T.jsx)(_,{defaultValue:`not-an-email`,className:`border-destructive`})}),(0,T.jsx)(a,{children:`Please enter a valid email address.`})]})})},k={name:`FormItem — Modified from Preset`,render:()=>(0,T.jsxs)(`div`,{className:`max-w-xs space-y-2`,children:[(0,T.jsxs)(l,{modified:!0,children:[(0,T.jsx)(d,{children:`Threshold`}),(0,T.jsx)(c,{children:`Minimum match score (0–100). Value differs from preset.`}),(0,T.jsx)(f,{children:(0,T.jsx)(_,{type:`number`,defaultValue:`85`})})]}),(0,T.jsxs)(l,{children:[(0,T.jsx)(d,{children:`Max Results`}),(0,T.jsx)(c,{children:`Unmodified field for comparison.`}),(0,T.jsx)(f,{children:(0,T.jsx)(_,{type:`number`,defaultValue:`100`})})]})]})},A={name:`FormItem — Disabled`,render:()=>(0,T.jsx)(`div`,{className:`max-w-xs`,children:(0,T.jsxs)(l,{disabled:!0,children:[(0,T.jsx)(d,{disabled:!0,children:`Output Path`}),(0,T.jsx)(c,{children:`Disabled when auto-detect is on.`}),(0,T.jsx)(f,{children:(0,T.jsx)(_,{defaultValue:`/output/report.html`,disabled:!0})})]})})},j={name:`FormItem — With Expandable Help`,render:()=>(0,T.jsx)(`div`,{className:`max-w-xs`,children:(0,T.jsxs)(l,{children:[(0,T.jsx)(d,{children:`Extraction Rules`}),(0,T.jsx)(f,{children:(0,T.jsx)(_,{defaultValue:`.*`,className:`font-mono`})}),(0,T.jsx)(o,{description:`Regex patterns that determine which JSON paths are extracted for translation.`,notes:[`Patterns are matched against the full JSON path (e.g., $.messages[*].text).`,`Use .* to extract all string values.`],dependencies:[{property:`extractAll`,condition:`must be false`}]})]})})},M={name:`FormToggle — Boolean Field`,render:()=>{let[e,t]=(0,w.useState)(!0);return(0,T.jsxs)(`div`,{className:`max-w-xs space-y-2`,children:[(0,T.jsx)(s,{checked:e,onCheckedChange:t,label:`Check Leading Whitespace`,description:`Flag text units where leading whitespace differs between source and target.`}),(0,T.jsx)(s,{checked:!1,onCheckedChange:()=>{},label:`Disabled Toggle`,description:`This toggle is disabled.`,disabled:!0}),(0,T.jsx)(s,{checked:!0,onCheckedChange:()=>{},label:`Modified Toggle`,description:`Value differs from active preset.`,modified:!0})]})}},N={name:`FormToggle — Compact Mode`,render:()=>{let[e,t]=(0,w.useState)(!0),[n,r]=(0,w.useState)(!1),[i,a]=(0,w.useState)(!0);return(0,T.jsxs)(`div`,{className:`max-w-xs space-y-0.5`,children:[(0,T.jsx)(s,{checked:e,onCheckedChange:t,label:`Leading whitespace`,compact:!0}),(0,T.jsx)(s,{checked:n,onCheckedChange:r,label:`Trailing whitespace`,compact:!0}),(0,T.jsx)(s,{checked:i,onCheckedChange:a,label:`Empty target`,compact:!0})]})}},P={name:`FormInputAction — Path Picker`,render:()=>(0,T.jsx)(`div`,{className:`max-w-sm`,children:(0,T.jsxs)(l,{children:[(0,T.jsx)(d,{children:`Report File Path`}),(0,T.jsx)(c,{children:`Path of the report file to generate.`}),(0,T.jsxs)(i,{children:[(0,T.jsx)(_,{defaultValue:"${rootDir}/qa-report.html",className:`flex-1 font-mono text-xs h-8`}),(0,T.jsx)(h,{variant:`outline`,size:`sm`,className:`h-8 text-xs shrink-0`,children:`Browse`})]})]})})},F={name:`FormInputAction — Multiple Actions`,render:()=>(0,T.jsx)(`div`,{className:`max-w-sm`,children:(0,T.jsxs)(l,{children:[(0,T.jsx)(d,{children:`TM File`}),(0,T.jsxs)(i,{children:[(0,T.jsx)(_,{defaultValue:`/data/project.tmx`,className:`flex-1 font-mono text-xs h-8`}),(0,T.jsx)(h,{variant:`outline`,size:`sm`,className:`h-8 text-xs shrink-0`,children:`Browse`}),(0,T.jsx)(h,{variant:`ghost`,size:`sm`,className:`h-8 text-xs shrink-0`,children:`Clear`})]})]})})},I={name:`FormFieldGroup — Non-Collapsible`,render:()=>(0,T.jsx)(`div`,{className:`max-w-xs`,children:(0,T.jsx)(u,{label:`General Settings`,children:(0,T.jsxs)(`div`,{className:`space-y-2`,children:[(0,T.jsxs)(l,{children:[(0,T.jsx)(d,{children:`Name`}),(0,T.jsx)(f,{children:(0,T.jsx)(_,{defaultValue:`My Project`})})]}),(0,T.jsxs)(l,{children:[(0,T.jsx)(d,{children:`Description`}),(0,T.jsx)(f,{children:(0,T.jsx)(_,{placeholder:`Optional description...`})})]})]})})})},L={name:`FormFieldGroup — Collapsible`,render:()=>(0,T.jsxs)(`div`,{className:`max-w-xs space-y-4`,children:[(0,T.jsx)(u,{label:`Whitespace Checks`,collapsible:!0,children:(0,T.jsxs)(`div`,{className:`space-y-1`,children:[(0,T.jsx)(s,{checked:!0,onCheckedChange:()=>{},label:`Leading whitespace`,compact:!0}),(0,T.jsx)(s,{checked:!0,onCheckedChange:()=>{},label:`Trailing whitespace`,compact:!0}),(0,T.jsx)(s,{checked:!1,onCheckedChange:()=>{},label:`Double spaces`,compact:!0})]})}),(0,T.jsx)(u,{label:`Content Checks`,collapsible:!0,defaultCollapsed:!0,children:(0,T.jsxs)(`div`,{className:`space-y-1`,children:[(0,T.jsx)(s,{checked:!0,onCheckedChange:()=>{},label:`Empty target`,compact:!0}),(0,T.jsx)(s,{checked:!0,onCheckedChange:()=>{},label:`Target same as source`,compact:!0})]})}),(0,T.jsx)(u,{label:`Output`,collapsible:!0,defaultCollapsed:!0,children:(0,T.jsx)(`div`,{className:`space-y-2`,children:(0,T.jsxs)(l,{children:[(0,T.jsx)(d,{children:`Report Path`}),(0,T.jsxs)(i,{children:[(0,T.jsx)(_,{defaultValue:"${rootDir}/qa-report.html",className:`flex-1 font-mono text-xs h-8`}),(0,T.jsx)(h,{variant:`outline`,size:`sm`,className:`h-8 text-xs shrink-0`,children:`Browse`})]})]})})})]})},R={name:`Composed — Mini Config Editor`,render:()=>{let[e,t]=(0,w.useState)(!1),[n,r]=(0,w.useState)(`95`),[a,o]=(0,w.useState)(`html`);return(0,T.jsxs)(`div`,{className:`max-w-sm space-y-4`,children:[(0,T.jsx)(u,{label:`Translation Memory`,children:(0,T.jsxs)(`div`,{className:`space-y-2`,children:[(0,T.jsx)(s,{checked:e,onCheckedChange:t,label:`Use Translation Memory`,description:`Leverage existing translations for pre-population.`}),(0,T.jsxs)(l,{disabled:!e,children:[(0,T.jsx)(d,{disabled:!e,children:`TM File`}),(0,T.jsxs)(i,{children:[(0,T.jsx)(_,{defaultValue:`/data/project.tmx`,disabled:!e,className:`flex-1 font-mono text-xs h-8`}),(0,T.jsx)(h,{variant:`outline`,size:`sm`,disabled:!e,className:`h-8 text-xs shrink-0`,children:`Browse`})]})]}),(0,T.jsxs)(l,{disabled:!e,children:[(0,T.jsx)(d,{disabled:!e,children:`Match Threshold`}),(0,T.jsx)(c,{children:`Minimum similarity score (0–100).`}),(0,T.jsx)(f,{children:(0,T.jsx)(_,{type:`number`,min:0,max:100,value:n,disabled:!e,className:`h-8 text-xs`,onChange:e=>r(e.target.value)})})]})]})}),(0,T.jsx)(u,{label:`Output`,collapsible:!0,children:(0,T.jsx)(`div`,{className:`space-y-2`,children:(0,T.jsxs)(l,{children:[(0,T.jsx)(d,{children:`Report Format`}),(0,T.jsx)(f,{children:(0,T.jsxs)(C,{value:a,onValueChange:o,children:[(0,T.jsx)(x,{className:`h-8 text-xs`,children:(0,T.jsx)(S,{})}),(0,T.jsxs)(b,{children:[(0,T.jsx)(y,{value:`html`,children:`HTML file`}),(0,T.jsx)(y,{value:`tsv`,children:`Tab-delimited file`}),(0,T.jsx)(y,{value:`xml`,children:`XML file`})]})]})})]})})})]})}},D.parameters={...D.parameters,docs:{...D.parameters?.docs,source:{originalSource:`{
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
}`,...D.parameters?.docs?.source}}},O.parameters={...O.parameters,docs:{...O.parameters?.docs,source:{originalSource:`{
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
}`,...O.parameters?.docs?.source}}},k.parameters={...k.parameters,docs:{...k.parameters?.docs,source:{originalSource:`{
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
}`,...k.parameters?.docs?.source}}},A.parameters={...A.parameters,docs:{...A.parameters?.docs,source:{originalSource:`{
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
}`,...A.parameters?.docs?.source}}},j.parameters={...j.parameters,docs:{...j.parameters?.docs,source:{originalSource:`{
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
}`,...j.parameters?.docs?.source}}},M.parameters={...M.parameters,docs:{...M.parameters?.docs,source:{originalSource:`{
  name: "FormToggle — Boolean Field",
  render: () => {
    const [checked, setChecked] = useState(true);
    return <div className="max-w-xs space-y-2">
        <FormToggle checked={checked} onCheckedChange={setChecked} label="Check Leading Whitespace" description="Flag text units where leading whitespace differs between source and target." />
        <FormToggle checked={false} onCheckedChange={() => {}} label="Disabled Toggle" description="This toggle is disabled." disabled />
        <FormToggle checked={true} onCheckedChange={() => {}} label="Modified Toggle" description="Value differs from active preset." modified />
      </div>;
  }
}`,...M.parameters?.docs?.source}}},N.parameters={...N.parameters,docs:{...N.parameters?.docs,source:{originalSource:`{
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
}`,...N.parameters?.docs?.source}}},P.parameters={...P.parameters,docs:{...P.parameters?.docs,source:{originalSource:`{
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
}`,...P.parameters?.docs?.source}}},F.parameters={...F.parameters,docs:{...F.parameters?.docs,source:{originalSource:`{
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
}`,...F.parameters?.docs?.source}}},I.parameters={...I.parameters,docs:{...I.parameters?.docs,source:{originalSource:`{
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
}`,...I.parameters?.docs?.source}}},L.parameters={...L.parameters,docs:{...L.parameters?.docs,source:{originalSource:`{
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
}`,...L.parameters?.docs?.source}}},R.parameters={...R.parameters,docs:{...R.parameters?.docs,source:{originalSource:`{
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
}`,...R.parameters?.docs?.source}}},z=[`BasicField`,`FieldWithError`,`FieldWithModifiedIndicator`,`DisabledField`,`FieldWithHelp`,`Toggle`,`ToggleCompact`,`InputWithAction`,`InputWithMultipleActions`,`FieldGroup`,`CollapsibleGroup`,`ComposedConfigEditor`]}))();export{D as BasicField,L as CollapsibleGroup,R as ComposedConfigEditor,A as DisabledField,I as FieldGroup,O as FieldWithError,j as FieldWithHelp,k as FieldWithModifiedIndicator,P as InputWithAction,F as InputWithMultipleActions,M as Toggle,N as ToggleCompact,z as __namedExportsOrder,E as default};