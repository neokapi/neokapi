import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./NotificationSettings-OwAsGp9B.js";var i,a,o,s,c,l,u,d,f;e((()=>{n(),i=t(),{fn:a}=__STORYBOOK_MODULE_TEST__,o={title:`Components/NotificationSettings`,component:r,tags:[`autodocs`],decorators:[e=>(0,i.jsx)(`div`,{style:{padding:24,maxWidth:560},children:(0,i.jsx)(e,{})})]},s={args:{settings:{frequency:`daily`,quiet_start:`22:00`,quiet_end:`08:00`,timezone:`America/New_York`},onChange:a()}},c={args:{settings:{frequency:`daily`,quiet_start:``,quiet_end:``,timezone:`UTC`},onChange:a()}},l={args:{settings:{frequency:`weekly`,quiet_start:`23:00`,quiet_end:`07:00`,timezone:`Europe/Berlin`},onChange:a()}},u={args:{settings:{frequency:`off`,quiet_start:``,quiet_end:``,timezone:`Asia/Tokyo`},onChange:a()}},d={args:{settings:{frequency:`daily`,quiet_start:`22:00`,quiet_end:`08:00`,timezone:`UTC`},onChange:a(),saving:!0}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    settings: {
      frequency: "daily",
      quiet_start: "22:00",
      quiet_end: "08:00",
      timezone: "America/New_York"
    },
    onChange: fn()
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    settings: {
      frequency: "daily",
      quiet_start: "",
      quiet_end: "",
      timezone: "UTC"
    },
    onChange: fn()
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    settings: {
      frequency: "weekly",
      quiet_start: "23:00",
      quiet_end: "07:00",
      timezone: "Europe/Berlin"
    },
    onChange: fn()
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    settings: {
      frequency: "off",
      quiet_start: "",
      quiet_end: "",
      timezone: "Asia/Tokyo"
    },
    onChange: fn()
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    settings: {
      frequency: "daily",
      quiet_start: "22:00",
      quiet_end: "08:00",
      timezone: "UTC"
    },
    onChange: fn(),
    saving: true
  }
}`,...d.parameters?.docs?.source}}},f=[`Default`,`QuietHoursOff`,`WeeklyDigest`,`DigestOff`,`Saving`]}))();export{s as Default,u as DigestOff,c as QuietHoursOff,d as Saving,l as WeeklyDigest,f as __namedExportsOrder,o as default};