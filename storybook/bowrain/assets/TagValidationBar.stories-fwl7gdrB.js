import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{t as n}from"./src-Bvgh6dl3.js";import{s as r}from"./TMGroupedEntry-CGOac2Ak.js";var i,a,o,s,c,l,u,d,f;e((()=>{n(),i=t(),a={title:`Editor/Tags/TagValidationBar`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{maxWidth:500,padding:16},children:(0,i.jsx)(e,{})})]},o={args:{validation:{valid:!0,errors:[],warnings:[]}}},s={args:{validation:{valid:!1,errors:[{type:`missing_tag`,message:`Missing 1 closing "b" tag`}],warnings:[]}}},c={args:{validation:{valid:!0,errors:[],warnings:[{type:`extra_tag`,message:`Extra 1 opening "i" tag`}]}}},l={args:{validation:{valid:!1,errors:[{type:`unpaired`,message:`Closing "b" without matching opening tag`}],warnings:[]}}},u={args:{validation:{valid:!1,errors:[{type:`missing_tag`,message:`Missing 1 opening "a" tag`},{type:`unpaired`,message:`1 opening "b" tags without matching closing tag`}],warnings:[{type:`extra_tag`,message:`Extra 2 placeholder "br" tags`}]}}},d={args:{validation:null}},o.parameters={...o.parameters,docs:{...o.parameters?.docs,source:{originalSource:`{
  args: {
    validation: {
      valid: true,
      errors: [],
      warnings: []
    }
  }
}`,...o.parameters?.docs?.source}}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    validation: {
      valid: false,
      errors: [{
        type: "missing_tag",
        message: 'Missing 1 closing "b" tag'
      }],
      warnings: []
    }
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    validation: {
      valid: true,
      errors: [],
      warnings: [{
        type: "extra_tag",
        message: 'Extra 1 opening "i" tag'
      }]
    }
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    validation: {
      valid: false,
      errors: [{
        type: "unpaired",
        message: 'Closing "b" without matching opening tag'
      }],
      warnings: []
    }
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    validation: {
      valid: false,
      errors: [{
        type: "missing_tag",
        message: 'Missing 1 opening "a" tag'
      }, {
        type: "unpaired",
        message: '1 opening "b" tags without matching closing tag'
      }],
      warnings: [{
        type: "extra_tag",
        message: 'Extra 2 placeholder "br" tags'
      }]
    } satisfies TagValidationResult
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    validation: null
  }
}`,...d.parameters?.docs?.source}}},f=[`Valid`,`MissingTag`,`ExtraTag`,`UnpairedTag`,`MultipleIssues`,`NoValidation`]}))();export{c as ExtraTag,s as MissingTag,u as MultipleIssues,d as NoValidation,l as UnpairedTag,o as Valid,f as __namedExportsOrder,a as default};