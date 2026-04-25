import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{_ as i,g as a}from"./iframe-BJVhYAGS.js";var o,s,c,l,u,d,f,p,m,h,g,_,v;t((()=>{o=e(n(),1),a(),s=r(),c={$id:`okf_html`,$version:`1.0`,title:`HTML Filter`,description:`Configures how HTML files are parsed for translatable content.`,type:`object`,formatMeta:{id:`okf_html`,extensions:[`.html`,`.htm`],mimeTypes:[`text/html`]},"ui:groups":[{id:`extraction`,label:`Extraction`,description:`Control which elements are extracted for translation.`,fields:[`extractMetaTitle`,`extractComments`,`extractAltText`]},{id:`advanced`,label:`Advanced`,description:`Advanced parsing options.`,collapsed:!0,fields:[`preserveWhitespace`,`maxSegmentLength`]}],properties:{extractMetaTitle:{type:`boolean`,description:`Extract the <title> meta tag for translation.`,default:!0},extractComments:{type:`boolean`,description:`Extract HTML comments as translatable content.`,default:!1},extractAltText:{type:`boolean`,description:`Extract alt attributes from image tags.`,default:!0},preserveWhitespace:{type:`boolean`,description:`Preserve original whitespace in extracted text.`,default:!1},maxSegmentLength:{type:`integer`,description:`Maximum segment length (0 = no limit).`,default:0,"ui:placeholder":`0`}}},l={$id:`okf_json`,$version:`1.47.0`,title:`JSON Filter`,description:`Configuration for the JSON filter. Use the inline code finder to detect markup patterns (e.g. HTML tags) within JSON string values.`,type:`object`,formatMeta:{id:`okf_json`,extensions:[`.json`,`.jsonc`],mimeTypes:[`application/json`]},"ui:groups":[{id:`extraction`,label:`Extraction`,description:`Controls which JSON values are extracted for translation.`,fields:[`extractAllPairs`,`extractionRules`]},{id:`inlineCodes`,label:`Inline Code Detection`,description:`Regex rules that identify inline markup within extracted blocks. Matched patterns become coded-text spans (opening, closing, or placeholder) that translators can reorder but not edit.`,fields:[`useCodeFinder`,`codeFinderRules`]}],properties:{extractAllPairs:{type:`boolean`,description:`Extract all key-value pairs as translatable content.`,default:!0},extractionRules:{type:`string`,description:`Extraction key patterns (regex). Leave empty to extract everything.`,default:``,"ui:widget":`regexBuilder`,"ui:placeholder":`e.g. ^(title|description|label)$`},useCodeFinder:{type:`boolean`,description:`Enable inline code detection. When on, patterns in the code finder rules are matched and converted to inline spans.`,default:!0},codeFinderRules:{type:`object`,description:`Regex patterns that identify inline markup within translatable blocks. Each rule defines a pattern; matched text becomes an inline code (span) in the editor.`,"ui:widget":`codeFinderRules`,"x-okapi-format":`inlineCodeFinder`,"ui:presets":{htmlTags:{rules:[{pattern:`</?[a-zA-Z][a-zA-Z0-9]*[^>]*/?>`},{pattern:`&[a-zA-Z]+;`}],sample:`Click <b>here</b> for &mdash; details`},markdownInline:{rules:[{pattern:`\\*\\*[^*]+\\*\\*`},{pattern:`\\*[^*]+\\*`},{pattern:"`[^`]+`"},{pattern:`\\[[^\\]]+\\]\\([^)]+\\)`}],sample:"Use **bold** or *italic* and `code` or [links](url)"},icuPlaceholders:{rules:[{pattern:`\\{[a-zA-Z_][a-zA-Z0-9_]*\\}`},{pattern:`\\{[a-zA-Z_][a-zA-Z0-9_]*, *(plural|select|selectordinal)[^}]*\\}`}],sample:`Hello {name}, you have {count, plural, one {# item} other {# items}}`}}}}},u={$id:`okf_plaintext`,$version:`1.47.0`,title:`Plain Text Filter`,description:`Configuration for plain-text files. The inline code finder detects markup or placeholder patterns embedded in text content.`,type:`object`,formatMeta:{id:`okf_plaintext`,extensions:[`.txt`,`.text`],mimeTypes:[`text/plain`]},"ui:groups":[{id:`general`,label:`General`,fields:[`lineBreakAsSegment`,`trimLeading`,`trimTrailing`]},{id:`inlineCodes`,label:`Inline Code Detection`,description:`Define regex patterns that identify inline codes (non-translatable tokens) within plain text. Detected patterns are protected as spans during translation.`,fields:[`useCodeFinder`,`codeFinderRules`]},{id:`encoding`,label:`Encoding`,collapsed:!0,fields:[`inputEncoding`,`outputEncoding`]}],properties:{lineBreakAsSegment:{type:`boolean`,description:`Treat each line break as a segment boundary.`,default:!0},trimLeading:{type:`boolean`,description:`Remove leading whitespace from each segment.`,default:!1},trimTrailing:{type:`boolean`,description:`Remove trailing whitespace from each segment.`,default:!0},useCodeFinder:{type:`boolean`,description:`Enable inline code pattern detection.`,default:!1},codeFinderRules:{type:`object`,description:`Regex rules for inline code detection. Matched patterns become non-translatable spans.`,"ui:widget":`codeFinderRules`,"x-okapi-format":`inlineCodeFinder`,"ui:presets":{variablePlaceholders:{rules:[{pattern:`\\$\\{[^}]+\\}`},{pattern:`%[sdfu%]`},{pattern:`%[0-9]*\\$[sdfu]`}],sample:"Hello ${user.name}, you scored %d out of %1$d"},xmlTags:{rules:[{pattern:`</?[a-zA-Z][a-zA-Z0-9]*[^>]*/?>`}],sample:`Click <link>here</link> to continue`}}},inputEncoding:{type:`string`,description:`Character encoding of the input file.`,default:`UTF-8`,"ui:placeholder":`UTF-8`},outputEncoding:{type:`string`,description:`Character encoding of the output file.`,default:`UTF-8`,"ui:placeholder":`UTF-8`}}},d={$id:`okf_xml`,$version:`1.47.0`,title:`XML Filter`,description:`Configuration for XML files. Supports inline code detection for mixed-content elements.`,type:`object`,formatMeta:{id:`okf_xml`,extensions:[`.xml`,`.resx`,`.svg`],mimeTypes:[`application/xml`,`text/xml`]},"ui:groups":[{id:`general`,label:`General`,fields:[`preserveWhitespace`,`extractNotes`]},{id:`inlineCodes`,label:`Inline Code Detection`,description:`Configure which XML elements and patterns are treated as inline codes rather than structural elements.`,fields:[`useCodeFinder`,`codeFinderRules`]},{id:`processing`,label:`Processing`,collapsed:!0,fields:[`escapeGT`,`collapseWhitespace`]}],properties:{preserveWhitespace:{type:`boolean`,description:`Preserve significant whitespace in text nodes.`,default:!1},extractNotes:{type:`boolean`,description:`Extract XML comments adjacent to translatable elements as notes.`,default:!0},useCodeFinder:{type:`boolean`,description:`Enable inline code detection within text content.`,default:!0},codeFinderRules:{type:`object`,description:`Patterns for detecting inline codes in mixed-content XML text nodes.`,"ui:widget":`codeFinderRules`,"x-okapi-format":`inlineCodeFinder`,"ui:presets":{commonInlineElements:{rules:[{pattern:`<(b|i|u|em|strong|code|a|span)[^>]*>`},{pattern:`</(b|i|u|em|strong|code|a|span)>`},{pattern:`<(br|hr|img)[^>]*/>`}],sample:`Click <b>here</b> to <a href="#">learn more</a>`},resnameVars:{rules:[{pattern:`\\{[0-9]+\\}`},{pattern:`\\%[0-9]*[sdfu]`}],sample:`Welcome {0}, you have {1} messages`}}},escapeGT:{type:`boolean`,description:`Escape > as &gt; in output.`,default:!1},collapseWhitespace:{type:`boolean`,description:`Collapse runs of whitespace into single spaces.`,default:!0}}},f={title:`Pages/Translation/FilterConfigEditor`,component:i,tags:[`autodocs`],parameters:{docs:{description:{component:"Schema-driven filter configuration editor. Each filter publishes a JSON Schema with `x-groups` for parameter grouping and `x-widget` hints for specialized UI controls. The `codeFinderRules` widget is the key control for configuring inline code (online code) detection — regex patterns that identify markup tokens within translatable text."}}},decorators:[e=>(0,s.jsx)(`div`,{style:{maxWidth:520,padding:24},children:(0,s.jsx)(e,{})})]},p={render:()=>{let[e,t]=(0,o.useState)({extractMetaTitle:!0,extractComments:!1,extractAltText:!0,preserveWhitespace:!1,maxSegmentLength:0});return(0,s.jsx)(i,{schema:c,value:e,onChange:t})}},m={render:()=>{let[e,t]=(0,o.useState)({extractAllPairs:!0,extractionRules:``,useCodeFinder:!0,codeFinderRules:{rules:[{pattern:`</?[a-zA-Z][a-zA-Z0-9]*[^>]*/?>`},{pattern:`&[a-zA-Z]+;`}],sample:`Click <b>here</b> for &mdash; details`}});return(0,s.jsx)(i,{schema:l,value:e,onChange:t})}},h={render:()=>{let[e,t]=(0,o.useState)({lineBreakAsSegment:!0,trimLeading:!1,trimTrailing:!0,useCodeFinder:!1,codeFinderRules:{rules:[],sample:``},inputEncoding:`UTF-8`,outputEncoding:`UTF-8`});return(0,s.jsx)(i,{schema:u,value:e,onChange:t})}},g={render:()=>{let[e,t]=(0,o.useState)({preserveWhitespace:!1,extractNotes:!0,useCodeFinder:!0,codeFinderRules:{rules:[{pattern:`<(b|i|u|em|strong|code|a|span)[^>]*>`},{pattern:`</(b|i|u|em|strong|code|a|span)>`},{pattern:`<(br|hr|img)[^>]*/>`}],sample:`Click <b>here</b> to <a href="#">learn more</a>`},escapeGT:!1,collapseWhitespace:!0});return(0,s.jsx)(i,{schema:d,value:e,onChange:t})}},_={render:()=>{let[e,t]=(0,o.useState)({extractAllPairs:!0,extractionRules:``,useCodeFinder:!0,codeFinderRules:{rules:[],sample:``}});return(0,s.jsx)(i,{schema:l,value:e,onChange:t})}},p.parameters={...p.parameters,docs:{...p.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [value, setValue] = useState<FilterParamsValue>({
      extractMetaTitle: true,
      extractComments: false,
      extractAltText: true,
      preserveWhitespace: false,
      maxSegmentLength: 0
    });
    return <FilterConfigEditor schema={htmlFilterSchema} value={value} onChange={setValue} />;
  }
}`,...p.parameters?.docs?.source},description:{story:`Basic HTML filter with boolean and integer fields, no inline code configuration.`,...p.parameters?.docs?.description}}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [value, setValue] = useState<FilterParamsValue>({
      extractAllPairs: true,
      extractionRules: "",
      useCodeFinder: true,
      codeFinderRules: {
        rules: [{
          pattern: "</?[a-zA-Z][a-zA-Z0-9]*[^>]*/?>"
        }, {
          pattern: "&[a-zA-Z]+;"
        }],
        sample: "Click <b>here</b> for &mdash; details"
      }
    });
    return <FilterConfigEditor schema={jsonFilterSchema} value={value} onChange={setValue} />;
  }
}`,...m.parameters?.docs?.source},description:{story:`JSON filter with **inline code detection** enabled.

Demonstrates the \`codeFinderRules\` widget with regex patterns for detecting
HTML tags within JSON string values. Use the **Presets** dropdown to load
preconfigured rule sets for HTML tags, Markdown inline syntax, or ICU placeholders.`,...m.parameters?.docs?.description}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [value, setValue] = useState<FilterParamsValue>({
      lineBreakAsSegment: true,
      trimLeading: false,
      trimTrailing: true,
      useCodeFinder: false,
      codeFinderRules: {
        rules: [],
        sample: ""
      },
      inputEncoding: "UTF-8",
      outputEncoding: "UTF-8"
    });
    return <FilterConfigEditor schema={plainTextFilterSchema} value={value} onChange={setValue} />;
  }
}`,...h.parameters?.docs?.source},description:{story:"Plain text filter with code finder **disabled** by default.\n\nToggle `useCodeFinder` on and add regex rules to detect embedded variables\nand formatting tokens in plain text files. Shows the `variablePlaceholders`\nand `xmlTags` presets.",...h.parameters?.docs?.description}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [value, setValue] = useState<FilterParamsValue>({
      preserveWhitespace: false,
      extractNotes: true,
      useCodeFinder: true,
      codeFinderRules: {
        rules: [{
          pattern: "<(b|i|u|em|strong|code|a|span)[^>]*>"
        }, {
          pattern: "</(b|i|u|em|strong|code|a|span)>"
        }, {
          pattern: "<(br|hr|img)[^>]*/>"
        }],
        sample: 'Click <b>here</b> to <a href="#">learn more</a>'
      },
      escapeGT: false,
      collapseWhitespace: true
    });
    return <FilterConfigEditor schema={xmlFilterSchema} value={value} onChange={setValue} />;
  }
}`,...g.parameters?.docs?.source},description:{story:"XML filter with inline code detection for mixed-content elements.\n\nDemonstrates the schema layout for XML-based formats where inline tags\n(e.g. `<b>`, `<a>`, `<br/>`) are treated as inline codes (spans) rather\nthan structural boundaries. Presets cover common inline HTML elements\nand numbered placeholders.",...g.parameters?.docs?.description}}},_.parameters={..._.parameters,docs:{..._.parameters?.docs,source:{originalSource:`{
  render: () => {
    const [value, setValue] = useState<FilterParamsValue>({
      extractAllPairs: true,
      extractionRules: "",
      useCodeFinder: true,
      codeFinderRules: {
        rules: [],
        sample: ""
      }
    });
    return <FilterConfigEditor schema={jsonFilterSchema} value={value} onChange={setValue} />;
  }
}`,..._.parameters?.docs?.source},description:{story:`Empty inline code rules — starting point for adding custom patterns.

Shows the code finder enabled but with no rules defined yet.
Click **+ Add Rule** to begin defining patterns.`,..._.parameters?.docs?.description}}},v=[`HTMLFilter`,`JSONFilterWithCodeFinder`,`PlainTextFilter`,`XMLFilterWithInlineCodes`,`EmptyCodeFinderRules`]}))();export{_ as EmptyCodeFinderRules,p as HTMLFilter,m as JSONFilterWithCodeFinder,h as PlainTextFilter,g as XMLFilterWithInlineCodes,v as __namedExportsOrder,f as default};