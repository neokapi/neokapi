import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./FlowEditor-DQA5B1FV.js";import{n as i,t as a}from"./tools-metadata-C20y-0MG.js";function o(e){return d[e]||null}function s(e){return f[e]||null}var c,l,u,d,f,p,m,h,g,_,v,y,b,x,S,C,w,T,E;e((()=>{n(),a(),c=t(),{fn:l}=__STORYBOOK_MODULE_TEST__,u=i,d={"pseudo-translate":{title:`Pseudo Translate`,type:`object`,toolMeta:{id:`pseudo-translate`,category:`transform`},"ui:groups":[{id:`output`,label:`Output Format`,fields:[`prefix`,`suffix`,`expansionPercent`]}],properties:{prefix:{type:`string`,default:`[`,description:`Prefix added to translations`},suffix:{type:`string`,default:`]`,description:`Suffix added to translations`},expansionPercent:{type:`integer`,default:30,minimum:0,maximum:200,description:`Expand text length %`},applyAccents:{type:`boolean`,default:!0,description:`Apply diacritical marks to characters`},padWithX:{type:`boolean`,default:!1,description:`Pad expansion with 'x' characters`}}},"qa-check":{title:`QA Check`,type:`object`,toolMeta:{id:`qa-check`,category:`validate`},"ui:groups":[{id:`checks`,label:`Enabled Checks`,fields:[`checkLeadingWhitespace`,`checkTrailingWhitespace`,`checkDoubleSpaces`,`checkMissingTranslation`]},{id:`codes`,label:`Code Checks`,fields:[`checkInlineCodes`,`checkPatterns`]}],properties:{checkLeadingWhitespace:{type:`boolean`,default:!0,description:`Check for leading whitespace mismatches`},checkTrailingWhitespace:{type:`boolean`,default:!0,description:`Check trailing whitespace`},checkDoubleSpaces:{type:`boolean`,default:!0,description:`Flag double spaces in target`},checkMissingTranslation:{type:`boolean`,default:!0,description:`Flag empty translations`},checkInlineCodes:{type:`boolean`,default:!0,description:`Verify inline codes are preserved`},checkPatterns:{type:`boolean`,default:!1,description:`Check for pattern mismatches`},severityLevel:{type:`string`,default:`warning`,enum:[`error`,`warning`,`info`],description:`Default severity`}}},"search-replace":{title:`Search and Replace`,type:`object`,toolMeta:{id:`search-replace`,category:`transform`},properties:{search:{type:`string`,description:`Search pattern`},replace:{type:`string`,description:`Replacement text`},regEx:{type:`boolean`,default:!1,description:`Use regular expressions`},target:{type:`boolean`,default:!0,description:`Apply to target text`},source:{type:`boolean`,default:!1,description:`Apply to source text`},dotAll:{type:`boolean`,default:!1,description:`Dot matches newlines`}}}},f={"pseudo-translate":{displayName:`Pseudo Translation`,overview:`Generates pseudo-translations by applying diacritical marks, padding, and brackets to source text. Useful for testing UI layout, detecting hardcoded strings, and verifying internationalization readiness without real translations.`,parameters:{prefix:{description:`Character(s) prepended to each translated string. Helps identify translated vs untranslated strings in the UI.`},suffix:{description:`Character(s) appended to each translated string.`},expansionPercent:{description:`Percentage to expand text length to simulate longer translations (e.g. German is ~30% longer than English).`,notes:[`Set to 0 to disable expansion. Values above 100% double the original length.`]},applyAccents:{description:`Replace ASCII characters with visually similar accented characters (e.g. a→á, e→é) to test rendering.`}},limitations:[`Does not handle right-to-left scripts.`,`Inline codes are preserved but not expanded.`],examples:[{title:`Basic pseudo`,description:`Default settings`,input:`Hello World`,output:`[Ĥéĺĺö Ŵöŕĺð]`}]},"qa-check":{displayName:`Quality Check`,overview:`Runs rule-based quality assurance checks on translations. Detects whitespace mismatches, missing translations, broken inline codes, and pattern inconsistencies between source and target.`,parameters:{checkLeadingWhitespace:{description:`Verify that leading whitespace in target matches source.`},checkInlineCodes:{description:`Verify all inline codes from source are preserved in target translation.`,notes:[`Inline codes include format specifiers ({0}), HTML tags, and printf patterns.`]},severityLevel:{description:`Default severity for issues found. Can be error, warning, or info.`}},processingNotes:[`Checks run independently — disabling one does not affect others.`,`Results are attached as annotations to each block.`]},"search-replace":{displayName:`Search and Replace`,overview:`Performs search and replace operations on source or target text. Supports both literal string matching and Java regular expressions.`,parameters:{search:{description:`The text or regex pattern to search for.`},replace:{description:`The replacement text. Supports $1, $2 backreferences when regex is enabled.`},regEx:{description:`When enabled, the search pattern is treated as a Java regular expression.`,notes:[`Use \\\\n for newline, \\\\t for tab in regex mode.`]}},wikiUrl:`https://okapiframework.org/wiki/index.php/Search_and_Replace_Step`}},p={title:`Flow Editor/FlowEditor`,component:r,tags:[`autodocs`],args:{onChange:l(),onRun:l(),onGetSchema:o,onGetDoc:s},parameters:{layout:`fullscreen`},decorators:[e=>(0,c.jsx)(`div`,{style:{height:700},children:(0,c.jsx)(e,{})})]},m={args:{flow:{steps:[{tool:`ai-translate`}]},tools:u}},h={args:{flow:{steps:[{tool:`ai-translate`},{tool:`qa-check`}]},tools:u}},g={args:{flow:{steps:[{tool:`tm-leverage`},{tool:`ai-translate`},{tool:`pseudo-translate`,config:{prefix:`>>`,suffix:`<<`}},{tool:`qa-check`},{tool:`word-count`}]},tools:u}},_={args:{flow:{steps:[{tool:`okapi:segmentation`},{tool:`okapi:leveraging`},{tool:`okapi:quality-check`}]},tools:u}},v={name:`Empty (Template Library)`,args:{flow:{steps:[]},tools:u}},y={args:{flow:{steps:[{tool:`ai-translate`},{tool:`qa-check`}]},tools:u,readOnly:!0,onRun:void 0}},b={args:{flow:{steps:[{tool:`pseudo-translate`,config:{prefix:`>>`,suffix:`<<`,expansionPercent:40}},{tool:`qa-check`,config:{checkLeadingWhitespace:!1}},{tool:`search-replace`,config:{search:`foo`,replace:`bar`,regEx:!1}}]},tools:u}},x={args:{flow:{steps:[{tool:`ai-translate`,label:`Translate`},{tool:``,parallel:[{tool:`qa-check`,label:`Quality Check`},{tool:`brand-vocab-check`,label:`Brand Check`}]},{tool:`word-count`,label:`Word Count`}]},tools:u}},S={args:{flow:{steps:[{tool:`tm-leverage`,label:`TM Lookup`},{tool:``,parallel:[{tool:`qa-check`,label:`QA`},{tool:`brand-vocab-check`,label:`Brand`},{tool:`entity-extract`,label:`Entities`}]}]},tools:u}},C={name:`Parallelization Suggestion`,args:{flow:{steps:[{tool:`ai-translate`},{tool:`qa-check`},{tool:`brand-vocab-check`},{tool:`word-count`}]},tools:u}},w={name:`With Port Visualization`,args:{flow:{steps:[{tool:`ai-translate`},{tool:`qa-check`},{tool:`word-count`}]},tools:u.map(e=>({...e,inputs:e.name===`ai-translate`||e.name===`qa-check`?[`block`]:[`block`,`data`],outputs:e.name===`ai-translate`||e.name===`qa-check`?[`block`]:[`data`]}))}},T={name:`With Trace (Completed)`,args:{flow:{steps:[{tool:`ai-translate`},{tool:`qa-check`},{tool:`word-count`}]},tools:u,readOnly:!0,onRun:void 0,traceEvents:[{ts:0,type:`enter`,nodeId:`tool-0`,partId:`p1`},{ts:500,type:`exit`,nodeId:`tool-0`,partId:`p1`},{ts:600,type:`enter`,nodeId:`tool-0`,partId:`p2`},{ts:900,type:`exit`,nodeId:`tool-0`,partId:`p2`},{ts:550,type:`enter`,nodeId:`tool-1`,partId:`p1`},{ts:1200,type:`exit`,nodeId:`tool-1`,partId:`p1`},{ts:950,type:`enter`,nodeId:`tool-1`,partId:`p2`},{ts:1800,type:`exit`,nodeId:`tool-1`,partId:`p2`},{ts:1250,type:`enter`,nodeId:`tool-2`,partId:`p1`},{ts:1400,type:`exit`,nodeId:`tool-2`,partId:`p1`},{ts:1850,type:`enter`,nodeId:`tool-2`,partId:`p2`},{ts:2e3,type:`exit`,nodeId:`tool-2`,partId:`p2`}],trace:{name:`translate-qa`,nodes:[{id:`tool-0`,type:`tool`,name:`ai-translate`},{id:`tool-1`,type:`tool`,name:`qa-check`},{id:`tool-2`,type:`tool`,name:`word-count`}],events:[],parts:{p1:{initial:{id:`p1`,type:`Block`,summary:`Hello world`,sourceText:`Hello world`},afterNode:{"tool-0":{id:`p1`,type:`Block`,summary:`Hello world`,sourceText:`Hello world`,targetText:`Bonjour le monde`},"tool-1":{id:`p1`,type:`Block`,summary:`Hello world`,sourceText:`Hello world`,targetText:`Bonjour le monde`},"tool-2":{id:`p1`,type:`Block`,summary:`Hello world`,sourceText:`Hello world`,targetText:`Bonjour le monde`}}},p2:{initial:{id:`p2`,type:`Block`,summary:`Click here`,sourceText:`Click here`},afterNode:{"tool-0":{id:`p2`,type:`Block`,summary:`Click here`,sourceText:`Click here`,targetText:`Cliquez ici`},"tool-1":{id:`p2`,type:`Block`,summary:`Click here`,sourceText:`Click here`,targetText:`Cliquez ici`},"tool-2":{id:`p2`,type:`Block`,summary:`Click here`,sourceText:`Click here`,targetText:`Cliquez ici`}}}},durationUs:2e3}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  args: {
    flow: {
      steps: [{
        tool: "ai-translate"
      }]
    },
    tools
  }
}`,...m.parameters?.docs?.source}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  args: {
    flow: {
      steps: [{
        tool: "ai-translate"
      }, {
        tool: "qa-check"
      }]
    },
    tools
  }
}`,...h.parameters?.docs?.source}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  args: {
    flow: {
      steps: [{
        tool: "tm-leverage"
      }, {
        tool: "ai-translate"
      }, {
        tool: "pseudo-translate",
        config: {
          prefix: ">>",
          suffix: "<<"
        }
      }, {
        tool: "qa-check"
      }, {
        tool: "word-count"
      }]
    },
    tools
  }
}`,...g.parameters?.docs?.source}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  args: {
    flow: {
      steps: [{
        tool: "okapi:segmentation"
      }, {
        tool: "okapi:leveraging"
      }, {
        tool: "okapi:quality-check"
      }]
    },
    tools
  }
}`,..._.parameters?.docs?.source}}},v.parameters={...v.parameters,docs:{...v.parameters?.docs,source:{originalSource:`{
  name: "Empty (Template Library)",
  args: {
    flow: {
      steps: []
    },
    tools
  }
}`,...v.parameters?.docs?.source}}},y.parameters={...y.parameters,docs:{...y.parameters?.docs,source:{originalSource:`{
  args: {
    flow: {
      steps: [{
        tool: "ai-translate"
      }, {
        tool: "qa-check"
      }]
    },
    tools,
    readOnly: true,
    onRun: undefined
  }
}`,...y.parameters?.docs?.source}}},b.parameters={...b.parameters,docs:{...b.parameters?.docs,source:{originalSource:`{
  args: {
    flow: {
      steps: [{
        tool: "pseudo-translate",
        config: {
          prefix: ">>",
          suffix: "<<",
          expansionPercent: 40
        }
      }, {
        tool: "qa-check",
        config: {
          checkLeadingWhitespace: false
        }
      }, {
        tool: "search-replace",
        config: {
          search: "foo",
          replace: "bar",
          regEx: false
        }
      }]
    },
    tools
  }
}`,...b.parameters?.docs?.source}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  args: {
    flow: {
      steps: [{
        tool: "ai-translate",
        label: "Translate"
      }, {
        tool: "",
        parallel: [{
          tool: "qa-check",
          label: "Quality Check"
        }, {
          tool: "brand-vocab-check",
          label: "Brand Check"
        }]
      }, {
        tool: "word-count",
        label: "Word Count"
      }]
    },
    tools
  }
}`,...x.parameters?.docs?.source}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  args: {
    flow: {
      steps: [{
        tool: "tm-leverage",
        label: "TM Lookup"
      }, {
        tool: "",
        parallel: [{
          tool: "qa-check",
          label: "QA"
        }, {
          tool: "brand-vocab-check",
          label: "Brand"
        }, {
          tool: "entity-extract",
          label: "Entities"
        }]
      }]
    },
    tools
  }
}`,...S.parameters?.docs?.source}}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  name: "Parallelization Suggestion",
  args: {
    flow: {
      steps: [{
        tool: "ai-translate"
      }, {
        tool: "qa-check"
      }, {
        tool: "brand-vocab-check"
      }, {
        tool: "word-count"
      }]
    },
    tools
  }
}`,...C.parameters?.docs?.source}}},w.parameters={...w.parameters,docs:{...w.parameters?.docs,source:{originalSource:`{
  name: "With Port Visualization",
  args: {
    flow: {
      steps: [{
        tool: "ai-translate"
      }, {
        tool: "qa-check"
      }, {
        tool: "word-count"
      }]
    },
    tools: tools.map(t => ({
      ...t,
      inputs: t.name === "ai-translate" ? ["block"] : t.name === "qa-check" ? ["block"] : ["block", "data"],
      outputs: t.name === "ai-translate" ? ["block"] : t.name === "qa-check" ? ["block"] : ["data"]
    }))
  }
}`,...w.parameters?.docs?.source}}},T.parameters={...T.parameters,docs:{...T.parameters?.docs,source:{originalSource:`{
  name: "With Trace (Completed)",
  args: {
    flow: {
      steps: [{
        tool: "ai-translate"
      }, {
        tool: "qa-check"
      }, {
        tool: "word-count"
      }]
    },
    tools,
    readOnly: true,
    onRun: undefined,
    traceEvents: [{
      ts: 0,
      type: "enter",
      nodeId: "tool-0",
      partId: "p1"
    }, {
      ts: 500,
      type: "exit",
      nodeId: "tool-0",
      partId: "p1"
    }, {
      ts: 600,
      type: "enter",
      nodeId: "tool-0",
      partId: "p2"
    }, {
      ts: 900,
      type: "exit",
      nodeId: "tool-0",
      partId: "p2"
    }, {
      ts: 550,
      type: "enter",
      nodeId: "tool-1",
      partId: "p1"
    }, {
      ts: 1200,
      type: "exit",
      nodeId: "tool-1",
      partId: "p1"
    }, {
      ts: 950,
      type: "enter",
      nodeId: "tool-1",
      partId: "p2"
    }, {
      ts: 1800,
      type: "exit",
      nodeId: "tool-1",
      partId: "p2"
    }, {
      ts: 1250,
      type: "enter",
      nodeId: "tool-2",
      partId: "p1"
    }, {
      ts: 1400,
      type: "exit",
      nodeId: "tool-2",
      partId: "p1"
    }, {
      ts: 1850,
      type: "enter",
      nodeId: "tool-2",
      partId: "p2"
    }, {
      ts: 2000,
      type: "exit",
      nodeId: "tool-2",
      partId: "p2"
    }],
    trace: {
      name: "translate-qa",
      nodes: [{
        id: "tool-0",
        type: "tool",
        name: "ai-translate"
      }, {
        id: "tool-1",
        type: "tool",
        name: "qa-check"
      }, {
        id: "tool-2",
        type: "tool",
        name: "word-count"
      }],
      events: [],
      parts: {
        p1: {
          initial: {
            id: "p1",
            type: "Block",
            summary: "Hello world",
            sourceText: "Hello world"
          },
          afterNode: {
            "tool-0": {
              id: "p1",
              type: "Block",
              summary: "Hello world",
              sourceText: "Hello world",
              targetText: "Bonjour le monde"
            },
            "tool-1": {
              id: "p1",
              type: "Block",
              summary: "Hello world",
              sourceText: "Hello world",
              targetText: "Bonjour le monde"
            },
            "tool-2": {
              id: "p1",
              type: "Block",
              summary: "Hello world",
              sourceText: "Hello world",
              targetText: "Bonjour le monde"
            }
          }
        },
        p2: {
          initial: {
            id: "p2",
            type: "Block",
            summary: "Click here",
            sourceText: "Click here"
          },
          afterNode: {
            "tool-0": {
              id: "p2",
              type: "Block",
              summary: "Click here",
              sourceText: "Click here",
              targetText: "Cliquez ici"
            },
            "tool-1": {
              id: "p2",
              type: "Block",
              summary: "Click here",
              sourceText: "Click here",
              targetText: "Cliquez ici"
            },
            "tool-2": {
              id: "p2",
              type: "Block",
              summary: "Click here",
              sourceText: "Click here",
              targetText: "Cliquez ici"
            }
          }
        }
      },
      durationUs: 2000
    }
  }
}`,...T.parameters?.docs?.source}}},E=[`SingleStep`,`MultiStep`,`FullPipeline`,`WithOkapiTools`,`EmptyWithTemplates`,`ReadOnly`,`WithConfiguration`,`ParallelBranches`,`ThreeWayParallel`,`ParallelizationSuggestion`,`WithPortVisualization`,`WithTraceData`]}))();export{v as EmptyWithTemplates,g as FullPipeline,h as MultiStep,x as ParallelBranches,C as ParallelizationSuggestion,y as ReadOnly,m as SingleStep,S as ThreeWayParallel,b as WithConfiguration,_ as WithOkapiTools,w as WithPortVisualization,T as WithTraceData,E as __namedExportsOrder,p as default};