import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./react-Baqbuk-D.js";import{t as n}from"./jsx-runtime-B-hFyic3.js";import{C as r,D as i,E as a,O as o,T as s,_ as c,b as l,c as u,d,f,h as p,k as m,l as h,m as g,n as _,r as v,t as y,u as b,v as x,w as S,x as C,y as w}from"./storybook-decorator-qg9t3HdX.js";var T,E,D,O,k,A,j,M,N,P,F,I,L,R,z,B=e((()=>{x(),t(),f(),T=n(),E=({frequency:e,totalUpdates:t,groups:n,settingsURL:f,dashboardURL:_})=>{let y=e===`weekly`?`Weekly Summary`:`Daily Digest`;return(0,T.jsxs)(r,{lang:`en`,dir:`ltr`,children:[(0,T.jsx)(a,{}),(0,T.jsxs)(C,{children:[y,` — `,t,` updates`]}),(0,T.jsx)(m,{style:p,children:(0,T.jsxs)(i,{style:D,children:[(0,T.jsxs)(l,{style:b,children:[(0,T.jsx)(w,{style:g,children:`Bowrain`}),(0,T.jsx)(w,{style:c,children:`Localization platform`})]}),(0,T.jsxs)(l,{style:O,children:[(0,T.jsx)(s,{as:`h1`,style:k,children:y}),(0,T.jsxs)(w,{style:A,children:[t,` new updates`]})]}),(0,T.jsxs)(l,{style:j,children:[n.map(e=>(0,T.jsxs)(l,{style:M,children:[(0,T.jsxs)(w,{style:N,children:[e.label,` (`,e.items.length,`)`]}),e.items.map((e,t)=>(0,T.jsxs)(l,{style:e.priority===`high`?{...P,...F}:P,children:[(0,T.jsx)(w,{style:I,children:e.title}),(0,T.jsx)(w,{style:L,children:e.body})]},t))]},e.category)),(0,T.jsx)(S,{style:d}),(0,T.jsx)(l,{style:R,children:(0,T.jsx)(o,{href:_,style:v,children:`Open Dashboard`})})]}),(0,T.jsxs)(l,{style:u,children:[(0,T.jsxs)(w,{style:h,children:[`You can change your digest frequency in `,(0,T.jsx)(`a`,{href:f,style:z,children:`notification settings`}),`.`]}),(0,T.jsx)(w,{style:h,children:`© Bowrain. All rights reserved.`})]})]})})]})},D={maxWidth:`600px`,margin:`40px auto`,padding:`0`},O={backgroundColor:`#1e293b`,padding:`20px 32px`},k={color:`#f8fafc`,fontSize:`20px`,fontWeight:`600`,margin:`0`},A={color:`#94a3b8`,fontSize:`14px`,margin:`4px 0 0`},j={backgroundColor:`#ffffff`,padding:`24px 32px 32px`},M={marginBottom:`20px`},N={fontSize:`13px`,fontWeight:`600`,color:`#64748b`,textTransform:`uppercase`,letterSpacing:`0.05em`,margin:`0 0 12px`,paddingBottom:`8px`,borderBottom:`1px solid #f1f5f9`},P={marginBottom:`12px`},F={borderLeft:`3px solid #ef4444`,paddingLeft:`12px`},I={margin:`0`,fontSize:`14px`,fontWeight:`500`,color:`#0f172a`},L={margin:`2px 0 0`,fontSize:`13px`,color:`#64748b`},R={textAlign:`center`,margin:`0`},z={color:`#2563eb`,textDecoration:`underline`},E.__docgenInfo={description:`Branded digest email for Bowrain.

In Storybook mode, groups are passed as a real array of DigestGroup.
In Go template mode, the entire body section is generated server-side
(the Go mailer iterates over notification groups). This component
serves as the visual reference and Storybook preview.`,methods:[],displayName:`DigestEmail`,props:{frequency:{required:!0,tsType:{name:`string`},description:`"daily" or "weekly"`},totalUpdates:{required:!0,tsType:{name:`string`},description:`Total number of notifications in this digest.`},groups:{required:!0,tsType:{name:`Array`,elements:[{name:`DigestGroup`}],raw:`DigestGroup[]`},description:`Grouped notification items, rendered as JSON in Go template mode.`},settingsURL:{required:!0,tsType:{name:`string`},description:`URL to notification settings.`},dashboardURL:{required:!0,tsType:{name:`string`},description:`URL to open the dashboard.`}}}})),V,H,U,W,G,K,q;e((()=>{B(),_(),V=n(),H={title:`Emails/Digest`,component:E,tags:[`autodocs`],parameters:{layout:`padded`},decorators:[(e,{args:t})=>(0,V.jsx)(y,{children:(0,V.jsx)(E,{...t})})]},U={args:{frequency:`daily`,totalUpdates:`7`,dashboardURL:`https://app.bowrain.com/ws/acme/notifications`,settingsURL:`https://app.bowrain.com/ws/acme/settings/notifications`,groups:[{category:`task`,label:`Tasks`,items:[{title:`New task assigned: Review French translations`,body:`Alice assigned you to review 24 blocks in fr-FR for the Mobile App project.`},{title:`Task completed: Translate homepage`,body:`Bob completed translating homepage content to de-DE.`}]},{category:`quality`,label:`Quality`,items:[{title:`Quality gate failed: Terminology check`,body:`3 terminology violations found in fr-FR for the Website project.`,priority:`high`}]},{category:`project`,label:`Project`,items:[{title:`Stream merged: feature/onboarding`,body:`The feature/onboarding stream was merged into main.`},{title:`New content available`,body:`12 new blocks pushed to the Mobile App project for translation.`}]},{category:`mention`,label:`Mentions`,items:[{title:`Alice mentioned you`,body:`"@charlie can you review the updated glossary terms?"`}]},{category:`automation`,label:`Automation`,items:[{title:`Flow failed: Auto-translate (ja-JP)`,body:`The auto-translate flow failed for ja-JP in the Website project.`,priority:`high`}]}]}},W={args:{frequency:`weekly`,totalUpdates:`23`,dashboardURL:`https://app.bowrain.com/ws/acme/notifications`,settingsURL:`https://app.bowrain.com/ws/acme/settings/notifications`,groups:[{category:`project`,label:`Project`,items:[{title:`Version v2.1 ready`,body:`All locales passed quality gates. Version v2.1 is ready for release.`},{title:`Stream merged: feature/payments`,body:`The feature/payments stream was merged into main.`},{title:`Progress milestone: fr-FR reached 100%`,body:`French translations are now complete for the Mobile App project.`},{title:`Progress milestone: ja-JP reached 75%`,body:`Japanese translations reached 75% completion for the Website project.`},{title:`New content available`,body:`48 new blocks pushed across 3 projects this week.`}]},{category:`task`,label:`Tasks`,items:[{title:`Deadline approaching: Translate settings page`,body:`Due in 24 hours. 8 blocks remaining in de-DE.`,priority:`high`},{title:`5 tasks completed this week`,body:`Translation and review tasks completed across Mobile App and Website.`},{title:`2 new tasks assigned`,body:`Review Korean translations, Translate checkout flow to pt-BR.`}]},{category:`quality`,label:`Quality`,items:[{title:`Quality gate resolved: Terminology check (fr-FR)`,body:`All terminology violations have been fixed.`},{title:`Quality gate failed: Length check (de-DE)`,body:`12 blocks exceed maximum length in de-DE for the Mobile App project.`,priority:`high`}]},{category:`mention`,label:`Mentions`,items:[{title:`Alice mentioned you (3 times)`,body:`Latest: "@charlie the Korean reviewer feedback is in"`},{title:`Bob mentioned you`,body:`"@charlie can you check the German plurals?"`}]},{category:`automation`,label:`Automation`,items:[{title:`6 automation runs completed`,body:`Auto-translate and quality check flows ran successfully.`},{title:`Flow failed: Quality check (ko-KR)`,body:`The quality check flow timed out for ko-KR. Manual retry needed.`,priority:`high`}]},{category:`system`,label:`System`,items:[{title:`New team member joined`,body:`Diana joined the workspace as a Translator.`}]}]}},G={args:{frequency:`daily`,totalUpdates:`1`,dashboardURL:`https://app.bowrain.com/ws/startup/notifications`,settingsURL:`https://app.bowrain.com/ws/startup/settings/notifications`,groups:[{category:`task`,label:`Tasks`,items:[{title:`New content available for translation`,body:`3 new blocks added to the Landing Page project.`}]}]}},K={args:{frequency:`daily`,totalUpdates:`3`,dashboardURL:`https://app.bowrain.com/ws/acme/notifications`,settingsURL:`https://app.bowrain.com/ws/acme/settings/notifications`,groups:[{category:`quality`,label:`Quality`,items:[{title:`Quality gate failed: Spelling check (es-ES)`,body:`18 spelling errors detected in the latest push.`,priority:`high`}]},{category:`task`,label:`Tasks`,items:[{title:`Deadline approaching: Review mobile content`,body:`Due tomorrow. 42 blocks awaiting review in ja-JP.`,priority:`high`}]},{category:`automation`,label:`Automation`,items:[{title:`Flow failed: Sync connector (GitHub)`,body:`GitHub connector sync failed. Authentication token may have expired.`,priority:`high`}]}]}},U.parameters={...U.parameters,docs:{...U.parameters?.docs,source:{originalSource:`{
  args: {
    frequency: "daily",
    totalUpdates: "7",
    dashboardURL: "https://app.bowrain.com/ws/acme/notifications",
    settingsURL: "https://app.bowrain.com/ws/acme/settings/notifications",
    groups: [{
      category: "task",
      label: "Tasks",
      items: [{
        title: "New task assigned: Review French translations",
        body: "Alice assigned you to review 24 blocks in fr-FR for the Mobile App project."
      }, {
        title: "Task completed: Translate homepage",
        body: "Bob completed translating homepage content to de-DE."
      }]
    }, {
      category: "quality",
      label: "Quality",
      items: [{
        title: "Quality gate failed: Terminology check",
        body: "3 terminology violations found in fr-FR for the Website project.",
        priority: "high"
      }]
    }, {
      category: "project",
      label: "Project",
      items: [{
        title: "Stream merged: feature/onboarding",
        body: "The feature/onboarding stream was merged into main."
      }, {
        title: "New content available",
        body: "12 new blocks pushed to the Mobile App project for translation."
      }]
    }, {
      category: "mention",
      label: "Mentions",
      items: [{
        title: "Alice mentioned you",
        body: '"@charlie can you review the updated glossary terms?"'
      }]
    }, {
      category: "automation",
      label: "Automation",
      items: [{
        title: "Flow failed: Auto-translate (ja-JP)",
        body: "The auto-translate flow failed for ja-JP in the Website project.",
        priority: "high"
      }]
    }]
  }
}`,...U.parameters?.docs?.source}}},W.parameters={...W.parameters,docs:{...W.parameters?.docs,source:{originalSource:`{
  args: {
    frequency: "weekly",
    totalUpdates: "23",
    dashboardURL: "https://app.bowrain.com/ws/acme/notifications",
    settingsURL: "https://app.bowrain.com/ws/acme/settings/notifications",
    groups: [{
      category: "project",
      label: "Project",
      items: [{
        title: "Version v2.1 ready",
        body: "All locales passed quality gates. Version v2.1 is ready for release."
      }, {
        title: "Stream merged: feature/payments",
        body: "The feature/payments stream was merged into main."
      }, {
        title: "Progress milestone: fr-FR reached 100%",
        body: "French translations are now complete for the Mobile App project."
      }, {
        title: "Progress milestone: ja-JP reached 75%",
        body: "Japanese translations reached 75% completion for the Website project."
      }, {
        title: "New content available",
        body: "48 new blocks pushed across 3 projects this week."
      }]
    }, {
      category: "task",
      label: "Tasks",
      items: [{
        title: "Deadline approaching: Translate settings page",
        body: "Due in 24 hours. 8 blocks remaining in de-DE.",
        priority: "high"
      }, {
        title: "5 tasks completed this week",
        body: "Translation and review tasks completed across Mobile App and Website."
      }, {
        title: "2 new tasks assigned",
        body: "Review Korean translations, Translate checkout flow to pt-BR."
      }]
    }, {
      category: "quality",
      label: "Quality",
      items: [{
        title: "Quality gate resolved: Terminology check (fr-FR)",
        body: "All terminology violations have been fixed."
      }, {
        title: "Quality gate failed: Length check (de-DE)",
        body: "12 blocks exceed maximum length in de-DE for the Mobile App project.",
        priority: "high"
      }]
    }, {
      category: "mention",
      label: "Mentions",
      items: [{
        title: "Alice mentioned you (3 times)",
        body: 'Latest: "@charlie the Korean reviewer feedback is in"'
      }, {
        title: "Bob mentioned you",
        body: '"@charlie can you check the German plurals?"'
      }]
    }, {
      category: "automation",
      label: "Automation",
      items: [{
        title: "6 automation runs completed",
        body: "Auto-translate and quality check flows ran successfully."
      }, {
        title: "Flow failed: Quality check (ko-KR)",
        body: "The quality check flow timed out for ko-KR. Manual retry needed.",
        priority: "high"
      }]
    }, {
      category: "system",
      label: "System",
      items: [{
        title: "New team member joined",
        body: "Diana joined the workspace as a Translator."
      }]
    }]
  }
}`,...W.parameters?.docs?.source}}},G.parameters={...G.parameters,docs:{...G.parameters?.docs,source:{originalSource:`{
  args: {
    frequency: "daily",
    totalUpdates: "1",
    dashboardURL: "https://app.bowrain.com/ws/startup/notifications",
    settingsURL: "https://app.bowrain.com/ws/startup/settings/notifications",
    groups: [{
      category: "task",
      label: "Tasks",
      items: [{
        title: "New content available for translation",
        body: "3 new blocks added to the Landing Page project."
      }]
    }]
  }
}`,...G.parameters?.docs?.source}}},K.parameters={...K.parameters,docs:{...K.parameters?.docs,source:{originalSource:`{
  args: {
    frequency: "daily",
    totalUpdates: "3",
    dashboardURL: "https://app.bowrain.com/ws/acme/notifications",
    settingsURL: "https://app.bowrain.com/ws/acme/settings/notifications",
    groups: [{
      category: "quality",
      label: "Quality",
      items: [{
        title: "Quality gate failed: Spelling check (es-ES)",
        body: "18 spelling errors detected in the latest push.",
        priority: "high"
      }]
    }, {
      category: "task",
      label: "Tasks",
      items: [{
        title: "Deadline approaching: Review mobile content",
        body: "Due tomorrow. 42 blocks awaiting review in ja-JP.",
        priority: "high"
      }]
    }, {
      category: "automation",
      label: "Automation",
      items: [{
        title: "Flow failed: Sync connector (GitHub)",
        body: "GitHub connector sync failed. Authentication token may have expired.",
        priority: "high"
      }]
    }]
  }
}`,...K.parameters?.docs?.source}}},q=[`DailyDigest`,`WeeklySummary`,`MinimalDaily`,`HighPriorityOnly`]}))();export{U as DailyDigest,K as HighPriorityOnly,G as MinimalDaily,W as WeeklySummary,q as __namedExportsOrder,H as default};