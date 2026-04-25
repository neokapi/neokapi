import{n as e}from"./chunk-DnJy8xQt.js";import{b as t,y as n}from"./iframe-Cr-Gz2Kj.js";var r,i,a,o,s,c,l,u,d,f;e((()=>{t(),r={title:`Billing/ModelUsageTable`,component:n,tags:[`autodocs`]},i=[{model:`claude-sonnet-4-20250514`,operation:`translate`,prompt_tokens:245e3,output_tokens:82e3,total_tokens:327e3,call_count:540},{model:`gpt-4o`,operation:`translate`,prompt_tokens:48e3,output_tokens:15e3,total_tokens:63e3,call_count:95},{model:`claude-sonnet-4-20250514`,operation:`qa_check`,prompt_tokens:38e3,output_tokens:9500,total_tokens:47500,call_count:72},{model:`claude-sonnet-4-20250514`,operation:`review`,prompt_tokens:12e3,output_tokens:4200,total_tokens:16200,call_count:18},{model:`gpt-4o`,operation:`entity_extract`,prompt_tokens:8400,output_tokens:3100,total_tokens:11500,call_count:12}],a=[{operation:`bravo_container`,total_seconds:2340,count:58},{operation:`auto_translate`,total_seconds:845,count:15},{operation:`auto_extract`,total_seconds:120,count:4}],o={args:{entries:i,runnerEntries:a}},s={args:{entries:[{model:`claude-sonnet-4-20250514`,operation:`translate`,prompt_tokens:12e5,output_tokens:38e4,total_tokens:158e4,call_count:2400}]}},c={args:{entries:[]}},l={args:{entries:[],runnerEntries:[{operation:`bravo_container`,total_seconds:7200,count:120},{operation:`auto_translate`,total_seconds:3600,count:45},{operation:`auto_extract`,total_seconds:180,count:6}]}},u={args:{entries:i,runnerEntries:a}},d={args:{entries:[{model:`claude-sonnet-4-20250514`,operation:`translate`,prompt_tokens:45e5,output_tokens:12e5,total_tokens:57e5,call_count:8500},{model:`gpt-4o`,operation:`translate`,prompt_tokens:21e5,output_tokens:65e4,total_tokens:275e4,call_count:4200},{model:`claude-sonnet-4-20250514`,operation:`qa_check`,prompt_tokens:89e4,output_tokens:21e4,total_tokens:11e5,call_count:1600},{model:`claude-sonnet-4-20250514`,operation:`brand_voice`,prompt_tokens:34e4,output_tokens:95e3,total_tokens:435e3,call_count:520}]}},o.parameters={...o.parameters,docs:{...o.parameters?.docs,source:{originalSource:`{
  args: {
    entries: sampleEntries,
    runnerEntries: sampleRunnerEntries
  }
}`,...o.parameters?.docs?.source}}},s.parameters={...s.parameters,docs:{...s.parameters?.docs,source:{originalSource:`{
  args: {
    entries: [{
      model: "claude-sonnet-4-20250514",
      operation: "translate",
      prompt_tokens: 1_200_000,
      output_tokens: 380_000,
      total_tokens: 1_580_000,
      call_count: 2_400
    }]
  }
}`,...s.parameters?.docs?.source}}},c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  args: {
    entries: []
  }
}`,...c.parameters?.docs?.source}}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    entries: [],
    runnerEntries: [{
      operation: "bravo_container",
      total_seconds: 7_200,
      count: 120
    }, {
      operation: "auto_translate",
      total_seconds: 3_600,
      count: 45
    }, {
      operation: "auto_extract",
      total_seconds: 180,
      count: 6
    }]
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    entries: sampleEntries,
    runnerEntries: sampleRunnerEntries
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    entries: [{
      model: "claude-sonnet-4-20250514",
      operation: "translate",
      prompt_tokens: 4_500_000,
      output_tokens: 1_200_000,
      total_tokens: 5_700_000,
      call_count: 8_500
    }, {
      model: "gpt-4o",
      operation: "translate",
      prompt_tokens: 2_100_000,
      output_tokens: 650_000,
      total_tokens: 2_750_000,
      call_count: 4_200
    }, {
      model: "claude-sonnet-4-20250514",
      operation: "qa_check",
      prompt_tokens: 890_000,
      output_tokens: 210_000,
      total_tokens: 1_100_000,
      call_count: 1_600
    }, {
      model: "claude-sonnet-4-20250514",
      operation: "brand_voice",
      prompt_tokens: 340_000,
      output_tokens: 95_000,
      total_tokens: 435_000,
      call_count: 520
    }]
  }
}`,...d.parameters?.docs?.source}}},f=[`Default`,`SingleModel`,`Empty`,`RunnerOnly`,`TokensAndRunner`,`HighVolume`]}))();export{o as Default,c as Empty,d as HighVolume,l as RunnerOnly,s as SingleModel,u as TokensAndRunner,f as __namedExportsOrder,r as default};