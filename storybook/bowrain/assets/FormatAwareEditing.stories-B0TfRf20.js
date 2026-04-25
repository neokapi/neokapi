import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./src-Q72DuJW2.js";import{l as i}from"./TMGroupedEntry-CGOac2Ak.js";import{n as a,t as o}from"./FormattedSourceDisplay-IUO0m4SR.js";import{n as s,t as c}from"./SourceCellDisplay-C_5x1VBh.js";import{n as l,t as u}from"./FormatVocabularyBadge-ByKA6B-5.js";function d({format:e,description:t,nativeExample:n,codedText:r,spans:i}){return(0,f.jsxs)(`div`,{style:N,children:[(0,f.jsxs)(`div`,{style:{display:`flex`,alignItems:`center`,justifyContent:`space-between`,marginBottom:8},children:[(0,f.jsxs)(`div`,{children:[(0,f.jsx)(`div`,{style:{fontSize:13,fontWeight:600},children:e}),(0,f.jsx)(`div`,{style:{fontSize:10,color:`#888`},children:t})]}),(0,f.jsx)(u,{spans:i})]}),(0,f.jsx)(`div`,{style:{fontSize:10,fontFamily:`monospace`,color:`#888`,marginBottom:8,padding:`4px 8px`,backgroundColor:`rgba(128,128,128,0.08)`,borderRadius:4,whiteSpace:`pre-wrap`},children:n}),(0,f.jsxs)(`div`,{style:{marginBottom:6},children:[(0,f.jsx)(`div`,{style:M,children:`Formatted view (what translators see)`}),(0,f.jsx)(`div`,{style:{fontSize:14,lineHeight:1.6},children:(0,f.jsx)(o,{codedText:r,spans:i})})]}),(0,f.jsxs)(`div`,{children:[(0,f.jsx)(`div`,{style:M,children:`Tag chip view (advanced)`}),(0,f.jsx)(`div`,{style:{fontSize:14,lineHeight:1.6},children:(0,f.jsx)(c,{codedText:r,spans:i})})]})]})}var f,p,m,h,g,_,v,y,b,x,S,C,w,T,E,D,O,k,A,j,M,N,P;e((()=>{a(),s(),r(),l(),f=t(),p=``,m=``,h=``,g=`Click ${p}here${m} to ${p}learn more${m}`,_=[{span_type:`opening`,type:`fmt:bold`,id:`1`,data:`<b class="cta">`},{span_type:`closing`,type:`fmt:bold`,id:`1`,data:`</b>`},{span_type:`opening`,type:`link:hyperlink`,id:`2`,data:`<a href="https://docs.example.com">`},{span_type:`closing`,type:`link:hyperlink`,id:`2`,data:`</a>`}],v=[{span_type:`opening`,type:`fmt:bold`,id:`1`,data:`**`},{span_type:`closing`,type:`fmt:bold`,id:`1`,data:`**`},{span_type:`opening`,type:`link:hyperlink`,id:`2`,data:`[`},{span_type:`closing`,type:`link:hyperlink`,id:`2`,data:`](https://docs.example.com)`}],y=[{span_type:`opening`,type:`fmt:bold`,id:`1`,data:`<pc id="1" dataRefStart="d1">`,sub_type:`xlf:b`},{span_type:`closing`,type:`fmt:bold`,id:`1`,data:`</pc>`,sub_type:`xlf:b`},{span_type:`opening`,type:`link:hyperlink`,id:`2`,data:`<pc id="2" dataRefStart="d2">`,sub_type:`xlf:a`},{span_type:`closing`,type:`link:hyperlink`,id:`2`,data:`</pc>`,sub_type:`xlf:a`}],b=`${p}Important:${m} Use ${p}kapi init${m} to get started.${h}See the ${p}docs${m} for more.`,x=[{span_type:`opening`,type:`fmt:bold`,id:`1`,data:`<strong>`},{span_type:`closing`,type:`fmt:bold`,id:`1`,data:`</strong>`},{span_type:`opening`,type:`fmt:code`,id:`2`,data:`<code>`},{span_type:`closing`,type:`fmt:code`,id:`2`,data:`</code>`},{span_type:`placeholder`,type:`struct:break`,id:`3`,data:`<br/>`},{span_type:`opening`,type:`link:hyperlink`,id:`4`,data:`<a href="/docs">`},{span_type:`closing`,type:`link:hyperlink`,id:`4`,data:`</a>`}],S=[{span_type:`opening`,type:`fmt:bold`,id:`1`,data:`**`},{span_type:`closing`,type:`fmt:bold`,id:`1`,data:`**`},{span_type:`opening`,type:`fmt:code`,id:`2`,data:"`"},{span_type:`closing`,type:`fmt:code`,id:`2`,data:"`"},{span_type:`placeholder`,type:`struct:break`,id:`3`,data:`  
`},{span_type:`opening`,type:`link:hyperlink`,id:`4`,data:`[`},{span_type:`closing`,type:`link:hyperlink`,id:`4`,data:`](/docs)`}],C=`Hello ${h}, you have ${h} new ${p}messages${m}.`,w=[{span_type:`placeholder`,type:`code:variable`,id:`1`,data:`{userName}`,display_text:`{userName}`,deletable:!1,cloneable:!1,can_reorder:!0},{span_type:`placeholder`,type:`code:placeholder`,id:`2`,data:`{count}`,display_text:`{count}`,deletable:!1,cloneable:!1,can_reorder:!0},{span_type:`opening`,type:`fmt:bold`,id:`3`,data:`<b>`},{span_type:`closing`,type:`fmt:bold`,id:`3`,data:`</b>`}],T={title:`Editor/Formatting/FormatAwareEditing`,tags:[`autodocs`],parameters:{docs:{description:{component:[`Demonstrates format-aware editing powered by vocabularies.`,``,`The same text appears identically in the editor regardless of whether it`,`came from an HTML file, a Markdown document, or an XLIFF exchange file.`,`This is because all three formats map to the same vocabulary types:`,``,'- HTML `<b>` = Markdown `**` = XLIFF `<pc dataRefStart="d1">` → **fmt:bold**','- HTML `<a>` = Markdown `[]()` = XLIFF `<pc dataRefStart="d2">` → **link:hyperlink**',``,`Translators see the same visual experience regardless of the source format.`].join(`
`)}}}},E={render:()=>(0,f.jsxs)(`div`,{style:{maxWidth:700,padding:16},children:[(0,f.jsx)(`h3`,{style:A,children:`Cross-Format Consistency`}),(0,f.jsx)(`p`,{style:j,children:`The same sentence extracted from HTML, Markdown, and XLIFF files. Despite completely different native syntax, the translator sees identical visual output because all three map to the same vocabulary types.`}),(0,f.jsxs)(`div`,{style:{display:`flex`,flexDirection:`column`,gap:16},children:[(0,f.jsx)(d,{format:`HTML`,description:`.html file`,nativeExample:`Click <b class="cta">here</b> to <a href="https://docs.example.com">learn more</a>`,codedText:g,spans:_}),(0,f.jsx)(d,{format:`Markdown`,description:`.md file`,nativeExample:`Click **here** to [learn more](https://docs.example.com)`,codedText:g,spans:v}),(0,f.jsx)(d,{format:`XLIFF 2.0`,description:`.xlf file`,nativeExample:`Click <pc id="1" dataRefStart="d1">here</pc> to <pc id="2" dataRefStart="d2">learn more</pc>`,codedText:g,spans:y})]})]})},D={render:()=>(0,f.jsxs)(`div`,{style:{maxWidth:700,padding:16},children:[(0,f.jsx)(`h3`,{style:A,children:`Rich Content: HTML vs Markdown`}),(0,f.jsx)(`p`,{style:j,children:`A more complex segment with bold, code, line break, and link. Both formats produce identical editor output.`}),(0,f.jsxs)(`div`,{style:{display:`flex`,gap:16},children:[(0,f.jsx)(`div`,{style:{flex:1},children:(0,f.jsx)(d,{format:`HTML`,description:`<strong>, <code>, <br/>, <a>`,nativeExample:`<strong>Important:</strong> Use <code>kapi init</code> to get started.<br/>See the <a href="/docs">docs</a>.`,codedText:b,spans:x})}),(0,f.jsx)(`div`,{style:{flex:1},children:(0,f.jsx)(d,{format:`Markdown`,description:"**, `, newline, []()",nativeExample:"**Important:** Use `kapi init` to get started.  \nSee the [docs](/docs).",codedText:b,spans:S})})]})]})},O={render:()=>(0,f.jsxs)(`div`,{style:{maxWidth:700,padding:16},children:[(0,f.jsx)(`h3`,{style:A,children:`Variables and Placeholders`}),(0,f.jsx)(`p`,{style:j,children:`i18n message formats embed variables and placeholders that must be preserved during translation. The vocabulary system marks these as non-deletable and non-cloneable, protecting them from accidental changes.`}),(0,f.jsx)(d,{format:`JSON i18n`,description:`ICU message format variables`,nativeExample:`{ "greeting": "Hello {userName}, you have {count} new <b>messages</b>." }`,codedText:C,spans:w}),(0,f.jsxs)(`div`,{style:{marginTop:16},children:[(0,f.jsx)(`div`,{style:M,children:`Tag palette (with constraint indicators)`}),(0,f.jsx)(i,{sourceSpans:w,onInsert:()=>{},showCategoryGroups:!0})]}),(0,f.jsxs)(`div`,{style:{marginTop:16},children:[(0,f.jsx)(`div`,{style:M,children:`Inline code legend`}),(0,f.jsx)(n,{spans:w,onClose:()=>{}})]})]})},k={render:()=>{let e=[...x,{span_type:`placeholder`,type:`code:variable`,id:`5`,data:`{version}`,display_text:`{version}`,deletable:!1,cloneable:!1}];return(0,f.jsxs)(`div`,{style:{maxWidth:640,padding:16},children:[(0,f.jsx)(`h3`,{style:A,children:`Tag Palette with Category Groups`}),(0,f.jsx)(`p`,{style:j,children:`When a segment contains tags from multiple vocabulary categories, the palette shows category separators for clarity.`}),(0,f.jsx)(i,{sourceSpans:e,onInsert:()=>{},showCategoryGroups:!0})]})}},A={fontSize:14,fontWeight:600,marginBottom:4},j={fontSize:12,color:`#888`,marginBottom:16,lineHeight:1.5},M={fontSize:10,fontWeight:600,color:`#888`,textTransform:`uppercase`,letterSpacing:`0.05em`,marginBottom:4},N={padding:16,borderRadius:8,border:`1px solid rgba(128,128,128,0.2)`,backgroundColor:`rgba(128,128,128,0.03)`},E.parameters={...E.parameters,docs:{...E.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    maxWidth: 700,
    padding: 16
  }}>
      <h3 style={titleStyle}>Cross-Format Consistency</h3>
      <p style={descStyle}>
        The same sentence extracted from HTML, Markdown, and XLIFF files. Despite completely
        different native syntax, the translator sees identical visual output because all three map
        to the same vocabulary types.
      </p>
      <div style={{
      display: "flex",
      flexDirection: "column",
      gap: 16
    }}>
        <FormatCard format="HTML" description=".html file" nativeExample={'Click <b class="cta">here</b> to <a href="https://docs.example.com">learn more</a>'} codedText={sentence} spans={htmlSpans} />
        <FormatCard format="Markdown" description=".md file" nativeExample="Click **here** to [learn more](https://docs.example.com)" codedText={sentence} spans={markdownSpans} />
        <FormatCard format="XLIFF 2.0" description=".xlf file" nativeExample={'Click <pc id="1" dataRefStart="d1">here</pc> to <pc id="2" dataRefStart="d2">learn more</pc>'} codedText={sentence} spans={xliffSpans} />
      </div>
    </div>
}`,...E.parameters?.docs?.source},description:{story:`Same sentence from three different formats renders identically.`,...E.parameters?.docs?.description}}},D.parameters={...D.parameters,docs:{...D.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    maxWidth: 700,
    padding: 16
  }}>
      <h3 style={titleStyle}>Rich Content: HTML vs Markdown</h3>
      <p style={descStyle}>
        A more complex segment with bold, code, line break, and link. Both formats produce identical
        editor output.
      </p>
      <div style={{
      display: "flex",
      gap: 16
    }}>
        <div style={{
        flex: 1
      }}>
          <FormatCard format="HTML" description="<strong>, <code>, <br/>, <a>" nativeExample={'<strong>Important:</strong> Use <code>kapi init</code> to get started.<br/>See the <a href="/docs">docs</a>.'} codedText={richSentence} spans={htmlRichSpans} />
        </div>
        <div style={{
        flex: 1
      }}>
          <FormatCard format="Markdown" description="**, \`, newline, []()" nativeExample={"**Important:** Use \`kapi init\` to get started.  \\nSee the [docs](/docs)."} codedText={richSentence} spans={mdRichSpans} />
        </div>
      </div>
    </div>
}`,...D.parameters?.docs?.source},description:{story:`Rich content with multiple tag types showing consistent treatment.`,...D.parameters?.docs?.description}}},O.parameters={...O.parameters,docs:{...O.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    maxWidth: 700,
    padding: 16
  }}>
      <h3 style={titleStyle}>Variables and Placeholders</h3>
      <p style={descStyle}>
        i18n message formats embed variables and placeholders that must be preserved during
        translation. The vocabulary system marks these as non-deletable and non-cloneable,
        protecting them from accidental changes.
      </p>
      <FormatCard format="JSON i18n" description="ICU message format variables" nativeExample={'{ "greeting": "Hello {userName}, you have {count} new <b>messages</b>." }'} codedText={i18nSentence} spans={i18nSpans} />
      <div style={{
      marginTop: 16
    }}>
        <div style={sectionLabel}>Tag palette (with constraint indicators)</div>
        <TagPalette sourceSpans={i18nSpans} onInsert={() => {}} showCategoryGroups />
      </div>
      <div style={{
      marginTop: 16
    }}>
        <div style={sectionLabel}>Inline code legend</div>
        <InlineCodeLegend spans={i18nSpans} onClose={() => {}} />
      </div>
    </div>
}`,...O.parameters?.docs?.source},description:{story:`Code tokens from i18n message formats (ICU, printf, etc.).`,...O.parameters?.docs?.description}}},k.parameters={...k.parameters,docs:{...k.parameters?.docs,source:{originalSource:`{
  render: () => {
    const mixedSpans: SpanInfo[] = [...htmlRichSpans, {
      span_type: "placeholder",
      type: "code:variable",
      id: "5",
      data: "{version}",
      display_text: "{version}",
      deletable: false,
      cloneable: false
    }];
    return <div style={{
      maxWidth: 640,
      padding: 16
    }}>
        <h3 style={titleStyle}>Tag Palette with Category Groups</h3>
        <p style={descStyle}>
          When a segment contains tags from multiple vocabulary categories, the palette shows
          category separators for clarity.
        </p>
        <TagPalette sourceSpans={mixedSpans} onInsert={() => {}} showCategoryGroups />
      </div>;
  }
}`,...k.parameters?.docs?.source},description:{story:`Tag palette with category separators for mixed-category spans.`,...k.parameters?.docs?.description}}},P=[`CrossFormatConsistency`,`RichContentAcrossFormats`,`CodeTokensAndVariables`,`TagPaletteWithCategories`]}))();export{O as CodeTokensAndVariables,E as CrossFormatConsistency,D as RichContentAcrossFormats,k as TagPaletteWithCategories,P as __namedExportsOrder,T as default};