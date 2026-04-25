import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./NotificationCenter-DJI8YKun.js";import{S as i,s as a}from"./fixtures-nZqLq3hB.js";var o,s,c,l,u,d,f;e((()=>{n(),a(),o=t(),{fn:s}=__STORYBOOK_MODULE_TEST__,c={title:`Components/NotificationCenter`,component:r,tags:[`autodocs`],decorators:[e=>(0,o.jsx)(`div`,{style:{padding:24},children:(0,o.jsx)(e,{})})]},l={args:{notifications:i,unreadCount:2,onMarkRead:s(),onMarkAllRead:s(),onDelete:s(),onNotificationClick:s()}},u={args:{notifications:i.map(e=>({...e,read:!0})),unreadCount:0,onMarkRead:s(),onMarkAllRead:s(),onDelete:s()}},d={args:{notifications:[],unreadCount:0,onMarkRead:s(),onMarkAllRead:s(),onDelete:s()}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    notifications: sampleNotifications,
    unreadCount: 2,
    onMarkRead: fn(),
    onMarkAllRead: fn(),
    onDelete: fn(),
    onNotificationClick: fn()
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    notifications: sampleNotifications.map(n => ({
      ...n,
      read: true
    })),
    unreadCount: 0,
    onMarkRead: fn(),
    onMarkAllRead: fn(),
    onDelete: fn()
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    notifications: [],
    unreadCount: 0,
    onMarkRead: fn(),
    onMarkAllRead: fn(),
    onDelete: fn()
  }
}`,...d.parameters?.docs?.source}}},f=[`WithUnread`,`AllRead`,`Empty`]}))();export{u as AllRead,d as Empty,l as WithUnread,f as __namedExportsOrder,c as default};