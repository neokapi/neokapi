import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{t as n}from"./src-Q72DuJW2.js";import{a as r,c as i,i as a,l as o,o as s,r as c,s as l}from"./selectable-list-C9LCxMFY.js";var u,d,f,p,m;e((()=>{n(),u=t(),d={title:`Foundations/Table`,component:c,tags:[`autodocs`],decorators:[e=>(0,u.jsx)(`div`,{style:{maxWidth:700,padding:16},children:(0,u.jsx)(e,{})})]},f=[{file:`messages.json`,locale:`fr-FR`,words:1243,status:`Translated`},{file:`messages.json`,locale:`de-DE`,words:1243,status:`In Review`},{file:`messages.json`,locale:`ja-JP`,words:1243,status:`Draft`},{file:`errors.json`,locale:`fr-FR`,words:312,status:`Translated`},{file:`errors.json`,locale:`de-DE`,words:312,status:`Not Started`},{file:`ui-labels.xliff`,locale:`es-ES`,words:876,status:`In Review`}],p={render:()=>(0,u.jsxs)(c,{children:[(0,u.jsx)(r,{children:`Translation file status overview`}),(0,u.jsx)(i,{children:(0,u.jsxs)(o,{children:[(0,u.jsx)(l,{children:`File`}),(0,u.jsx)(l,{children:`Locale`}),(0,u.jsx)(l,{className:`text-right`,children:`Words`}),(0,u.jsx)(l,{children:`Status`})]})}),(0,u.jsx)(a,{children:f.map((e,t)=>(0,u.jsxs)(o,{children:[(0,u.jsx)(s,{className:`font-medium`,children:e.file}),(0,u.jsx)(s,{children:e.locale}),(0,u.jsx)(s,{className:`text-right`,children:e.words.toLocaleString()}),(0,u.jsx)(s,{children:e.status})]},t))})]})},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  render: () => <Table>
      <TableCaption>Translation file status overview</TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead>File</TableHead>
          <TableHead>Locale</TableHead>
          <TableHead className="text-right">Words</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {files.map((row, i) => <TableRow key={i}>
            <TableCell className="font-medium">{row.file}</TableCell>
            <TableCell>{row.locale}</TableCell>
            <TableCell className="text-right">{row.words.toLocaleString()}</TableCell>
            <TableCell>{row.status}</TableCell>
          </TableRow>)}
      </TableBody>
    </Table>
}`,...p.parameters?.docs?.source}}},m=[`Default`]}))();export{p as Default,m as __namedExportsOrder,d as default};