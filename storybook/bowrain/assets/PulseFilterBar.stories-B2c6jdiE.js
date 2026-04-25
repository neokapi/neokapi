import{n as e}from"./chunk-DnJy8xQt.js";import{o as t,r as n}from"./iframe-Cr-Gz2Kj.js";var r,i,a,o,s;e((()=>{n(),r={title:`Pulse/PulseFilterBar`,component:t,tags:[`autodocs`],parameters:{layout:`padded`}},i={args:{filters:[{key:`language`,value:`fr-FR`},{key:`project`,value:`Web Application`}],onRemove:e=>console.log(`remove`,e),onClear:()=>console.log(`clear`)}},a={args:{filters:[],onRemove:e=>console.log(`remove`,e),onClear:()=>console.log(`clear`),presets:[{label:`This week`,filters:[{key:`time`,value:`this-week`}]},{label:`Needs help`,filters:[{key:`progress`,value:`<50`}]}]}},o={args:{filters:[{key:`language`,value:`fr-FR`},{key:`language`,value:`de-DE`},{key:`project`,value:`Web Application`},{key:`contributor`,value:`Alice Chen`},{key:`period`,value:`last-30-days`}],onRemove:e=>console.log(`remove`,e),onClear:()=>console.log(`clear`)}},i.parameters={...i.parameters,docs:{...i.parameters?.docs,source:{originalSource:`{
  args: {
    filters: [{
      key: "language",
      value: "fr-FR"
    }, {
      key: "project",
      value: "Web Application"
    }],
    onRemove: (key: string) => console.log("remove", key),
    onClear: () => console.log("clear")
  }
}`,...i.parameters?.docs?.source}}},a.parameters={...a.parameters,docs:{...a.parameters?.docs,source:{originalSource:`{
  args: {
    filters: [],
    onRemove: (key: string) => console.log("remove", key),
    onClear: () => console.log("clear"),
    presets: [{
      label: "This week",
      filters: [{
        key: "time",
        value: "this-week"
      }]
    }, {
      label: "Needs help",
      filters: [{
        key: "progress",
        value: "<50"
      }]
    }]
  }
}`,...a.parameters?.docs?.source}}},o.parameters={...o.parameters,docs:{...o.parameters?.docs,source:{originalSource:`{
  args: {
    filters: [{
      key: "language",
      value: "fr-FR"
    }, {
      key: "language",
      value: "de-DE"
    }, {
      key: "project",
      value: "Web Application"
    }, {
      key: "contributor",
      value: "Alice Chen"
    }, {
      key: "period",
      value: "last-30-days"
    }],
    onRemove: (key: string) => console.log("remove", key),
    onClear: () => console.log("clear")
  }
}`,...o.parameters?.docs?.source}}},s=[`WithFilters`,`Empty`,`ManyFilters`]}))();export{a as Empty,o as ManyFilters,i as WithFilters,s as __namedExportsOrder,r as default};