import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{t as i}from"./src-Q72DuJW2.js";import{n as a}from"./schema-form-ZmYkxXQr.js";function o({schema:e,initialValues:t={},compact:n=!1,width:r=360,presetValues:i}){let[o,l]=(0,s.useState)(t);return(0,c.jsxs)(`div`,{style:{maxWidth:r},children:[(0,c.jsx)(a,{schema:e,values:o,onChange:l,compact:n,presetValues:i}),(0,c.jsx)(`pre`,{style:{marginTop:16,padding:12,borderRadius:6,background:`oklch(0.17 0.012 260)`,fontSize:11,color:`oklch(0.7 0.01 260)`,overflow:`auto`,maxHeight:200},children:JSON.stringify(o,null,2)})]})}var s,c,l,u,d,f,p,m,h,g,_,v,y,b,x,S,C,w,T,E,D,O,k,A,j,M,N,P,F,I,L;t((()=>{s=e(n(),1),i(),c=r(),l={title:`Pseudo Translate`,type:`object`,toolMeta:{id:`pseudo-translate`,category:`transform`},"ui:groups":[{id:`output`,label:`Output Format`,fields:[`prefix`,`suffix`,`expansionPercent`]},{id:`behavior`,label:`Behavior`,fields:[`applyAccents`,`padWithX`]}],properties:{prefix:{type:`string`,title:`Prefix`,default:`[`,description:`Prefix added to translations`},suffix:{type:`string`,title:`Suffix`,default:`]`,description:`Suffix added to translations`},expansionPercent:{type:`integer`,title:`Expansion Percent`,default:30,minimum:0,maximum:200,description:`Expand text length by percentage`},applyAccents:{type:`boolean`,title:`Apply Accents`,default:!0,description:`Apply diacritical marks to simulate translated text`},padWithX:{type:`boolean`,title:`Pad with X`,default:!1,description:`Pad expansion with 'x' characters instead of spaces`}}},u={title:`QA Check`,type:`object`,toolMeta:{id:`qa-check`,category:`validate`},"ui:groups":[{id:`whitespace`,label:`Whitespace Checks`,fields:[`checkLeadingWhitespace`,`checkTrailingWhitespace`,`checkDoubleSpaces`]},{id:`content`,label:`Content Checks`,fields:[`checkMissingTranslation`,`checkInlineCodes`,`checkPatterns`]}],properties:{checkLeadingWhitespace:{type:`boolean`,title:`Check Leading Whitespace`,default:!0,description:`Check for leading whitespace mismatches`},checkTrailingWhitespace:{type:`boolean`,title:`Check Trailing Whitespace`,default:!0,description:`Check trailing whitespace`},checkDoubleSpaces:{type:`boolean`,title:`Check Double Spaces`,default:!0,description:`Flag double spaces in target`},checkMissingTranslation:{type:`boolean`,title:`Check Missing Translation`,default:!0,description:`Flag empty translations`},checkInlineCodes:{type:`boolean`,title:`Check Inline Codes`,default:!0,description:`Verify inline codes are preserved`},checkPatterns:{type:`boolean`,title:`Check Patterns`,default:!1,description:`Check for pattern mismatches`},severityLevel:{type:`string`,title:`Severity Level`,default:`warning`,enum:[`error`,`warning`,`info`],description:`Default severity level`},maxIssues:{type:`integer`,title:`Max Issues`,default:100,minimum:1,maximum:1e4,description:`Maximum issues to report`}}},d={title:`Search and Replace`,type:`object`,toolMeta:{id:`search-replace`,category:`transform`},properties:{search:{type:`string`,title:`Search`,description:`Search pattern or literal text`},replace:{type:`string`,title:`Replace`,description:`Replacement text`},regEx:{type:`boolean`,title:`Regular Expression`,default:!1,description:`Treat search as a regular expression`},target:{type:`boolean`,title:`Apply to Target`,default:!0,description:`Apply to target text`},source:{type:`boolean`,title:`Apply to Source`,default:!1,description:`Apply to source text`},dotAll:{type:`boolean`,title:`Dot All`,default:!1,description:`Dot matches newlines in regex`},caseInsensitive:{type:`boolean`,title:`Case Insensitive`,default:!1,description:`Case-insensitive matching`}}},f={title:`HTML Filter`,type:`object`,formatMeta:{id:`html-filter`},"ui:groups":[{id:`parser`,label:`Parser Settings`,fields:[`parser`]},{id:`extraction`,label:`Extraction`,fields:[`elements`,`attributes`]},{id:`inline`,label:`Inline Codes`,fields:[`inlineCodes`]}],properties:{parser:{type:`object`,title:`Parser`,description:`Parser behavior settings`,properties:{assumeWellformed:{type:`boolean`,title:`Assume Wellformed`,default:!1,description:`Assume input HTML is well-formed XML. Faster but may fail on non-conforming HTML.`},preserveWhitespace:{type:`boolean`,title:`Preserve Whitespace`,default:!1,description:`Preserve original whitespace in extracted text`}}},elements:{type:`object`,title:`Elements`,description:`Element extraction rules`,"ui:widget":`elementRulesEditor`,additionalProperties:{type:`object`,properties:{ruleTypes:{type:`array`,title:`Rule Types`,items:{type:`string`,enum:[`INLINE`,`TEXTUNIT`,`EXCLUDE`,`INCLUDE`,`ATTRIBUTES_ONLY`]}}}}},attributes:{type:`object`,title:`Attributes`,description:`Global attribute extraction rules`,"ui:widget":`attributeRulesEditor`,additionalProperties:{type:`object`,properties:{ruleTypes:{type:`array`,title:`Rule Types`,items:{type:`string`,enum:[`ATTRIBUTE_TRANS`,`ATTRIBUTE_WRITABLE`,`ATTRIBUTE_READONLY`,`ATTRIBUTE_ID`]}},allElementsExcept:{type:`array`,title:`All Elements Except`,items:{type:`string`}}}}},inlineCodes:{type:`object`,title:`Inline Codes`,description:`Inline code detection and handling`,"ui:widget":`codeFinderRules`,"ui:presets":{"HTML Tags":{rules:[{pattern:`</?\\w[^>]*>`}]},"Printf Placeholders":{rules:[{pattern:`%[\\w.]*[dsfx]`}]}}},editorTitle:{type:`string`,title:`Editor Title`,description:`Display title in editor`},simplifierRules:{type:`string`,title:`Simplifier Rules`,"ui:widget":`simplifierRulesEditor`,description:`Rules for simplifying inline code representation`}}},p={title:`Fixed-Width Columns`,type:`object`,formatMeta:{id:`fixed-width`},properties:{columns:{type:`array`,title:`Columns`,description:`Column definitions for fixed-width parsing`,items:{type:`object`,properties:{name:{type:`string`,title:`Name`,description:`Column name`},start:{type:`integer`,title:`Start`,minimum:0,description:`Start position (0-based)`},width:{type:`integer`,title:`Width`,minimum:1,description:`Column width`},translatable:{type:`boolean`,title:`Translatable`,default:!1,description:`Whether to extract for translation`}}}},encoding:{type:`string`,title:`Encoding`,default:`UTF-8`,enum:[`UTF-8`,`UTF-16`,`ISO-8859-1`,`Windows-1252`],description:`File character encoding`},skipLines:{type:`integer`,title:`Skip Lines`,default:0,minimum:0,description:`Header lines to skip`}}},m={title:`Regex Filter`,type:`object`,formatMeta:{id:`regex-filter`},properties:{patterns:{type:`array`,title:`Patterns`,description:`Extraction patterns (regex)`,items:{type:`string`}},caseSensitive:{type:`boolean`,title:`Case Sensitive`,default:!0}}},h={title:`Custom Config`,type:`object`,toolMeta:{id:`custom`,category:`pipeline`},properties:{name:{type:`string`,title:`Name`,description:`Configuration name`},settings:{type:`object`,title:`Settings`,description:`Arbitrary settings (JSON)`},tags:{type:`array`,title:`Tags`,description:`Tags for this configuration`}}},g={title:`Conditional Fields`,type:`object`,toolMeta:{id:`conditional`,category:`transform`},properties:{mode:{type:`string`,title:`Mode`,default:`simple`,enum:[`simple`,`advanced`],description:`Processing mode`},threshold:{type:`number`,title:`Threshold`,default:.8,description:`Match threshold (only in advanced mode)`,"ui:visible":{field:`mode`,eq:`advanced`}},maxResults:{type:`integer`,title:`Max Results`,default:100,description:`Maximum results (only in advanced mode)`,"ui:visible":{field:`mode`,eq:`advanced`}},caseSensitive:{type:`boolean`,title:`Case Sensitive`,default:!1,description:`Case-sensitive matching`}}},_={title:`HTML Format`,type:`object`,"ui:groups":[{id:`parser`,label:`Parser Settings`,fields:[`preserveWhitespace`]},{id:`extraction`,label:`Extraction Rules`,fields:[`elements`,`attributes`]},{id:`codes`,label:`Inline Codes`,fields:[`codeFinderRules`,`useCodeFinder`]}],properties:{preserveWhitespace:{type:`boolean`,title:`Preserve Whitespace`,default:!1,description:`Preserve significant whitespace in text nodes`},elements:{type:`object`,title:`Elements`,description:`Map of element names to extraction rules`,additionalProperties:{type:`object`,properties:{ruleType:{type:`string`,title:`Rule Type`,enum:[`INLINE`,`GROUP`,`EXCLUDE`,`TEXTUNIT`,`PRESERVE_WHITESPACE`],default:`INLINE`},translatable:{type:`boolean`,title:`Translatable`,default:!0}}}},attributes:{type:`object`,title:`Attributes`,description:`Map of attribute names to extraction rules`,additionalProperties:{type:`string`}},codeFinderRules:{type:`object`,title:`Code Finder Rules`,description:`Rules for identifying inline codes`,properties:{useAllRulesWhenTesting:{type:`boolean`,title:`Use All Rules When Testing`,default:!0},includes:{type:`array`,title:`Includes`,items:{type:`string`},description:`Regex patterns to include as inline codes`},excludes:{type:`array`,title:`Excludes`,items:{type:`string`},description:`Regex patterns to exclude from inline codes`}}},useCodeFinder:{type:`boolean`,title:`Use Code Finder`,default:!0,description:`Enable the inline code finder`}}},v={title:`Environment Variables`,type:`object`,toolMeta:{id:`env-vars`,category:`config`},"ui:groups":[{id:`maps`,label:`Variable Maps`,fields:[`variables`,`secrets`,`overrides`]},{id:`options`,label:`Options`,fields:[`expandVars`,`caseSensitiveKeys`]}],properties:{variables:{type:`object`,title:`Variables`,description:`Environment variables (key-value pairs)`,additionalProperties:{type:`string`}},secrets:{type:`object`,title:`Secrets`,description:`Secret variables (masked in output)`,additionalProperties:{type:`string`}},overrides:{type:`object`,title:`Overrides`,description:`Per-locale variable overrides`,additionalProperties:{type:`object`,properties:{value:{type:`string`,title:`Value`,description:`Override value`},locales:{type:`string`,title:`Locales`,description:`Comma-separated locale list`}}}},expandVars:{type:`boolean`,title:`Expand Vars`,default:!0,description:"Expand ${VAR} references in values"},caseSensitiveKeys:{type:`boolean`,title:`Case Sensitive Keys`,default:!0,description:`Treat variable names as case-sensitive`}}},y={title:`JSON Format`,type:`object`,formatMeta:{id:`json-format`},"ui:groups":[{id:`parsing`,label:`Parsing`,fields:[`keyStyle`,`arrayHandling`,`preserveOrder`]},{id:`extraction`,label:`Extraction`,fields:[`extractPaths`,`excludePaths`]},{id:`output`,label:`Output`,fields:[`indentation`,`trailingNewline`,`sortKeys`]}],properties:{keyStyle:{type:`string`,title:`Key Style`,default:`nested`,enum:[`nested`,`flat`,`auto`],description:`How to interpret JSON key paths`},arrayHandling:{type:`string`,title:`Array Handling`,default:`index`,enum:[`index`,`content`,`skip`],description:`How to handle array elements`},preserveOrder:{type:`boolean`,title:`Preserve Order`,default:!0,description:`Preserve original key order in output`},extractPaths:{type:`array`,title:`Extract Paths`,items:{type:`string`},description:`JSON paths to extract (empty = extract all)`},excludePaths:{type:`array`,title:`Exclude Paths`,items:{type:`string`},description:`JSON paths to exclude from extraction`},indentation:{type:`integer`,title:`Indentation`,default:2,minimum:0,maximum:8,description:`Number of spaces for indentation`},trailingNewline:{type:`boolean`,title:`Trailing Newline`,default:!0,description:`Add trailing newline to output`},sortKeys:{type:`boolean`,title:`Sort Keys`,default:!1,description:`Alphabetically sort keys in output`}}},b={title:`Flow Editor/SchemaForm`,component:o,tags:[`autodocs`],parameters:{layout:`padded`}},x={args:{schema:l}},S={args:{schema:l,initialValues:{prefix:`>>`,suffix:`<<`,expansionPercent:45,applyAccents:!1}}},C={args:{schema:u}},w={args:{schema:d}},T={args:{schema:u,compact:!0}},E={name:`Nested Object (HTML Filter)`,args:{schema:f}},D={name:`Nested Object (with values)`,args:{schema:f,initialValues:{parser:{assumeWellformed:!0,preserveWhitespace:!1},elements:{meta:{ruleTypes:[`ATTRIBUTES_ONLY`]},script:{ruleTypes:[`EXCLUDE`]}},inlineCodes:{rules:[{pattern:`</?\\w[^>]*>`}],sample:`<b>bold</b> text`}}}},O={name:`Array of Objects (Fixed-Width)`,args:{schema:p,initialValues:{columns:[{name:`id`,start:0,width:10,translatable:!1},{name:`text`,start:10,width:50,translatable:!0}]}}},k={name:`Array of Strings (Regex)`,args:{schema:m,initialValues:{patterns:[`^\\s*msgid\\s*"(.*)"`,`^\\s*msgstr\\s*"(.*)"`]}}},A={name:`JSON Fallback (bare object/array)`,args:{schema:h,initialValues:{name:`custom`,settings:{timeout:30,debug:!0},tags:[`production`,`v2`]}}},j={name:`ui:visible Conditional Visibility`,args:{schema:g,initialValues:{mode:`simple`}}},M={name:`ui:visible (advanced mode)`,args:{schema:g,initialValues:{mode:`advanced`,threshold:.75,maxResults:50}}},N={name:`Deeply Nested Config (HTML Format)`,args:{schema:_,initialValues:{preserveWhitespace:!1,elements:{div:{ruleType:`TEXTUNIT`,translatable:!0},span:{ruleType:`INLINE`,translatable:!0},script:{ruleType:`EXCLUDE`,translatable:!1}},attributes:{title:`translatable`,alt:`translatable`,placeholder:`translatable`},codeFinderRules:{useAllRulesWhenTesting:!0,includes:[`</?\\w[^>]*>`,`\\{\\{.*?\\}\\}`],excludes:[`<!--.*?-->`]},useCodeFinder:!0}}},P={name:`Map Editor (key-value maps)`,args:{schema:v,initialValues:{variables:{API_URL:`https://api.example.com`,APP_NAME:`My App`,VERSION:`2.1.0`},secrets:{API_KEY:`sk-***`},overrides:{},expandVars:!0,caseSensitiveKeys:!0}}},F={name:`Formats Page Config (wide)`,args:{schema:y,width:500,initialValues:{keyStyle:`nested`,arrayHandling:`index`,preserveOrder:!0,extractPaths:[`$.messages`,`$.labels`],excludePaths:[`$.internal`],indentation:2,trailingNewline:!0,sortKeys:!1}}},I={name:`With Preset Indicator`,args:{schema:l,initialValues:{prefix:`>>`,suffix:`]`,expansionPercent:45,applyAccents:!0,padWithX:!1},presetValues:{prefix:`[`,suffix:`]`,expansionPercent:30,applyAccents:!0,padWithX:!1}}},x.parameters={...x.parameters,docs:{...x.parameters?.docs,source:{originalSource:`{
  args: {
    schema: pseudoTranslateSchema
  }
}`,...x.parameters?.docs?.source}}},S.parameters={...S.parameters,docs:{...S.parameters?.docs,source:{originalSource:`{
  args: {
    schema: pseudoTranslateSchema,
    initialValues: {
      prefix: ">>",
      suffix: "<<",
      expansionPercent: 45,
      applyAccents: false
    }
  }
}`,...S.parameters?.docs?.source}}},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  args: {
    schema: qaCheckSchema
  }
}`,...C.parameters?.docs?.source}}},w.parameters={...w.parameters,docs:{...w.parameters?.docs,source:{originalSource:`{
  args: {
    schema: searchReplaceSchema
  }
}`,...w.parameters?.docs?.source}}},T.parameters={...T.parameters,docs:{...T.parameters?.docs,source:{originalSource:`{
  args: {
    schema: qaCheckSchema,
    compact: true
  }
}`,...T.parameters?.docs?.source}}},E.parameters={...E.parameters,docs:{...E.parameters?.docs,source:{originalSource:`{
  name: "Nested Object (HTML Filter)",
  args: {
    schema: nestedObjectSchema
  }
}`,...E.parameters?.docs?.source}}},D.parameters={...D.parameters,docs:{...D.parameters?.docs,source:{originalSource:`{
  name: "Nested Object (with values)",
  args: {
    schema: nestedObjectSchema,
    initialValues: {
      parser: {
        assumeWellformed: true,
        preserveWhitespace: false
      },
      elements: {
        meta: {
          ruleTypes: ["ATTRIBUTES_ONLY"]
        },
        script: {
          ruleTypes: ["EXCLUDE"]
        }
      },
      inlineCodes: {
        rules: [{
          pattern: "</?\\\\w[^>]*>"
        }],
        sample: "<b>bold</b> text"
      }
    }
  }
}`,...D.parameters?.docs?.source}}},O.parameters={...O.parameters,docs:{...O.parameters?.docs,source:{originalSource:`{
  name: "Array of Objects (Fixed-Width)",
  args: {
    schema: arraySchema,
    initialValues: {
      columns: [{
        name: "id",
        start: 0,
        width: 10,
        translatable: false
      }, {
        name: "text",
        start: 10,
        width: 50,
        translatable: true
      }]
    }
  }
}`,...O.parameters?.docs?.source}}},k.parameters={...k.parameters,docs:{...k.parameters?.docs,source:{originalSource:`{
  name: "Array of Strings (Regex)",
  args: {
    schema: simpleArraySchema,
    initialValues: {
      patterns: ['^\\\\s*msgid\\\\s*"(.*)"', '^\\\\s*msgstr\\\\s*"(.*)"']
    }
  }
}`,...k.parameters?.docs?.source}}},A.parameters={...A.parameters,docs:{...A.parameters?.docs,source:{originalSource:`{
  name: "JSON Fallback (bare object/array)",
  args: {
    schema: jsonFallbackSchema,
    initialValues: {
      name: "custom",
      settings: {
        timeout: 30,
        debug: true
      },
      tags: ["production", "v2"]
    }
  }
}`,...A.parameters?.docs?.source}}},j.parameters={...j.parameters,docs:{...j.parameters?.docs,source:{originalSource:`{
  name: "ui:visible Conditional Visibility",
  args: {
    schema: showIfSchema,
    initialValues: {
      mode: "simple"
    }
  }
}`,...j.parameters?.docs?.source}}},M.parameters={...M.parameters,docs:{...M.parameters?.docs,source:{originalSource:`{
  name: "ui:visible (advanced mode)",
  args: {
    schema: showIfSchema,
    initialValues: {
      mode: "advanced",
      threshold: 0.75,
      maxResults: 50
    }
  }
}`,...M.parameters?.docs?.source}}},N.parameters={...N.parameters,docs:{...N.parameters?.docs,source:{originalSource:`{
  name: "Deeply Nested Config (HTML Format)",
  args: {
    schema: deeplyNestedSchema,
    initialValues: {
      preserveWhitespace: false,
      elements: {
        div: {
          ruleType: "TEXTUNIT",
          translatable: true
        },
        span: {
          ruleType: "INLINE",
          translatable: true
        },
        script: {
          ruleType: "EXCLUDE",
          translatable: false
        }
      },
      attributes: {
        title: "translatable",
        alt: "translatable",
        placeholder: "translatable"
      },
      codeFinderRules: {
        useAllRulesWhenTesting: true,
        includes: ["</?\\\\w[^>]*>", "\\\\{\\\\{.*?\\\\}\\\\}"],
        excludes: ["<!--.*?-->"]
      },
      useCodeFinder: true
    }
  }
}`,...N.parameters?.docs?.source}}},P.parameters={...P.parameters,docs:{...P.parameters?.docs,source:{originalSource:`{
  name: "Map Editor (key-value maps)",
  args: {
    schema: mapEditorSchema,
    initialValues: {
      variables: {
        API_URL: "https://api.example.com",
        APP_NAME: "My App",
        VERSION: "2.1.0"
      },
      secrets: {
        API_KEY: "sk-***"
      },
      overrides: {},
      expandVars: true,
      caseSensitiveKeys: true
    }
  }
}`,...P.parameters?.docs?.source}}},F.parameters={...F.parameters,docs:{...F.parameters?.docs,source:{originalSource:`{
  name: "Formats Page Config (wide)",
  args: {
    schema: formatsPageSchema,
    width: 500,
    initialValues: {
      keyStyle: "nested",
      arrayHandling: "index",
      preserveOrder: true,
      extractPaths: ["$.messages", "$.labels"],
      excludePaths: ["$.internal"],
      indentation: 2,
      trailingNewline: true,
      sortKeys: false
    }
  }
}`,...F.parameters?.docs?.source}}},I.parameters={...I.parameters,docs:{...I.parameters?.docs,source:{originalSource:`{
  name: "With Preset Indicator",
  args: {
    schema: pseudoTranslateSchema,
    initialValues: {
      prefix: ">>",
      suffix: "]",
      expansionPercent: 45,
      applyAccents: true,
      padWithX: false
    },
    presetValues: {
      prefix: "[",
      suffix: "]",
      expansionPercent: 30,
      applyAccents: true,
      padWithX: false
    }
  }
}`,...I.parameters?.docs?.source}}},L=[`PseudoTranslate`,`PseudoTranslateWithValues`,`QACheck`,`SearchReplace`,`Compact`,`NestedObject`,`NestedObjectWithValues`,`ArrayOfObjects`,`SimpleArray`,`JsonFallback`,`ConditionalFields`,`ConditionalFieldsAdvanced`,`DeeplyNestedConfig`,`MapEditorStory`,`FormatsPageConfig`,`WithPresetIndicator`]}))();export{O as ArrayOfObjects,T as Compact,j as ConditionalFields,M as ConditionalFieldsAdvanced,N as DeeplyNestedConfig,F as FormatsPageConfig,A as JsonFallback,P as MapEditorStory,E as NestedObject,D as NestedObjectWithValues,x as PseudoTranslate,S as PseudoTranslateWithValues,C as QACheck,w as SearchReplace,k as SimpleArray,I as WithPresetIndicator,L as __namedExportsOrder,b as default};