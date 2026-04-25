import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,t as a}from"./PluralTargetEditor-BjcuO6vO.js";function o(e){return()=>{let[t,n]=(0,s.useState)(e);return(0,c.jsx)(a,{block:u,target:t,onChange:n})}}var s,c,l,u,d,f,p,m,h,g,_,v;t((()=>{s=e(n(),1),i(),c=r(),l={title:`Editor/Plural/PluralTargetEditor`,component:a,tags:[`autodocs`],decorators:[e=>(0,c.jsx)(`div`,{style:{maxWidth:640,padding:16},children:(0,c.jsx)(e,{})})],parameters:{docs:{description:{component:"Translator-facing editor for a single locale's target. When the source is a flat sentence but the target locale needs plural handling, the translator can upgrade the target into a `PluralRun` without touching the source. Switches back and forth between flat textarea and per-form textareas via the same component."}}}},u={source:[{text:`You have `},{ph:{id:`1`,type:`jsx:var`,data:`{count}`,equiv:`count`}},{text:` messages`}],placeholders:[{name:`count`,kind:`variable`,jsType:`number`,sourceExpr:`count`}]},d=[{text:`Sie haben `},{ph:{id:`1`,type:`jsx:var`,data:`{count}`,equiv:`count`}},{text:` Nachrichten`}],f=[{plural:{pivot:`count`,forms:{zero:[{text:`Keine Nachrichten`}],one:[{text:`1 Nachricht`}],other:d}}}],p={name:`Flat target (upgrade available)`,render:o(d)},m={name:`Empty target (new locale)`,render:o([])},h={name:`Plural target (downgrade available)`,render:o(f)},g={name:`Plural target partially filled`,render:o([{plural:{pivot:`count`,forms:{zero:[],one:[{text:`1 Nachricht`}],other:[]}}}])},_={name:`Block with multiple placeholder candidates`,render:()=>{let e={source:[{text:`User `},{ph:{id:`1`,type:`jsx:var`,data:`{name}`,equiv:`name`}},{text:` opened `},{ph:{id:`2`,type:`jsx:var`,data:`{count}`,equiv:`count`}},{text:` files in `},{ph:{id:`3`,type:`jsx:var`,data:`{folder}`,equiv:`folder`}}],placeholders:[{name:`name`,kind:`variable`,jsType:`string`,sourceExpr:`user.name`},{name:`count`,kind:`variable`,jsType:`number`,sourceExpr:`file.count`},{name:`folder`,kind:`variable`,jsType:`string`,sourceExpr:`folder.name`}]},[t,n]=(0,s.useState)([{text:`Benutzer `},{ph:{id:`1`,type:`jsx:var`,data:`{name}`,equiv:`name`}},{text:` hat `},{ph:{id:`2`,type:`jsx:var`,data:`{count}`,equiv:`count`}},{text:` Dateien geöffnet in `},{ph:{id:`3`,type:`jsx:var`,data:`{folder}`,equiv:`folder`}}]);return(0,c.jsx)(a,{block:e,target:t,onChange:n})}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  name: "Flat target (upgrade available)",
  render: Interactive(flatGerman)
}`,...p.parameters?.docs?.source}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "Empty target (new locale)",
  render: Interactive([])
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  name: "Plural target (downgrade available)",
  render: Interactive(pluralGerman)
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  name: "Plural target partially filled",
  render: Interactive([{
    plural: {
      pivot: "count",
      forms: {
        zero: [],
        one: [{
          text: "1 Nachricht"
        }],
        other: []
      }
    }
  }])
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  name: "Block with multiple placeholder candidates",
  render: () => {
    const richBlock: Pick<Block, "source" | "placeholders"> = {
      source: [{
        text: "User "
      }, {
        ph: {
          id: "1",
          type: "jsx:var",
          data: "{name}",
          equiv: "name"
        }
      }, {
        text: " opened "
      }, {
        ph: {
          id: "2",
          type: "jsx:var",
          data: "{count}",
          equiv: "count"
        }
      }, {
        text: " files in "
      }, {
        ph: {
          id: "3",
          type: "jsx:var",
          data: "{folder}",
          equiv: "folder"
        }
      }],
      placeholders: [{
        name: "name",
        kind: "variable",
        jsType: "string",
        sourceExpr: "user.name"
      }, {
        name: "count",
        kind: "variable",
        jsType: "number",
        sourceExpr: "file.count"
      }, {
        name: "folder",
        kind: "variable",
        jsType: "string",
        sourceExpr: "folder.name"
      }]
    };
    const [target, setTarget] = useState<Run[]>([{
      text: "Benutzer "
    }, {
      ph: {
        id: "1",
        type: "jsx:var",
        data: "{name}",
        equiv: "name"
      }
    }, {
      text: " hat "
    }, {
      ph: {
        id: "2",
        type: "jsx:var",
        data: "{count}",
        equiv: "count"
      }
    }, {
      text: " Dateien geöffnet in "
    }, {
      ph: {
        id: "3",
        type: "jsx:var",
        data: "{folder}",
        equiv: "folder"
      }
    }]);
    return <PluralTargetEditor block={richBlock} target={target} onChange={setTarget} />;
  }
}`,..._.parameters?.docs?.source}}},v=[`FlatTarget`,`EmptyFlat`,`FullPlural`,`PluralPartiallyFilled`,`MultiplePlaceholders`]}))();export{m as EmptyFlat,p as FlatTarget,h as FullPlural,_ as MultiplePlaceholders,g as PluralPartiallyFilled,v as __namedExportsOrder,l as default};