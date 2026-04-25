import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./src-Q72DuJW2.js";import{n as i,t as a}from"./FormattedSourceDisplay-IUO0m4SR.js";import{n as o,t as s}from"./SourceCellDisplay-C_5x1VBh.js";import{n as c,t as l}from"./FormatVocabularyBadge-ByKA6B-5.js";import{B as u,C as d,E as f,G as p,H as m,K as h,S as g,T as _,U as v,V as y,W as b,_ as x,a as S,b as C,c as w,g as T,h as E,i as D,l as O,m as k,q as A,x as j}from"./fixtures-D8OXoGLB.js";function M({title:e,children:t}){return(0,P.jsxs)(`div`,{style:{marginBottom:24},children:[(0,P.jsx)(`h3`,{style:{fontSize:11,fontWeight:600,textTransform:`uppercase`,letterSpacing:`0.08em`,color:`var(--sb-color, #888)`,marginBottom:8,borderBottom:`1px solid var(--sb-border, #333)`,paddingBottom:4},children:e}),t]})}function N({label:e,codedText:t,spans:n}){return(0,P.jsxs)(`div`,{style:{marginBottom:16},children:[(0,P.jsx)(`div`,{style:{fontSize:10,color:`#888`,fontWeight:600,marginBottom:4},children:e}),(0,P.jsx)(`div`,{style:{fontSize:14,lineHeight:1.6},children:(0,P.jsx)(a,{codedText:t,spans:n})})]})}var P,F,I,L,R,z,B,V,H,U,W,G;e((()=>{i(),o(),c(),r(),O(),P=t(),F=``,I=``,L=``,R={title:`Editor/Formatting/FormatInlineCodes`,tags:[`autodocs`],parameters:{docs:{description:{component:[`Shows how inline codes are rendered in the default formatted view.`,``,`- **Formatted view** (default): text appears with actual formatting — bold is bold, links are underlined, code is monospace — with faint background tints marking span boundaries.`,"- **Code view** (opt-in): text shows abstract tag chips like `[B>] text [/B]` for advanced users who need to see the raw structure.",``,`HTML and Markdown spans produce identical visual output because both resolve to the same semantic categories (bold, italic, link, code, etc.).`].join(`
`)}}}},z={render:()=>(0,P.jsx)(`div`,{style:{maxWidth:640,padding:16},children:(0,P.jsxs)(M,{title:`HTML Spans — Formatted View`,children:[(0,P.jsx)(N,{label:`Bold`,codedText:u,spans:y}),(0,P.jsx)(N,{label:`Link + Italic`,codedText:T,spans:x}),(0,P.jsx)(N,{label:`Inline Code`,codedText:D,spans:S}),(0,P.jsx)(N,{label:`Line Break`,codedText:k,spans:E})]})})},B={render:()=>(0,P.jsx)(`div`,{style:{maxWidth:640,padding:16},children:(0,P.jsxs)(M,{title:`Markdown Spans — Formatted View`,children:[(0,P.jsx)(N,{label:`Bold + Italic (** and *)`,codedText:C,spans:j}),(0,P.jsx)(N,{label:"Code + Link (` and []())",codedText:g,spans:d})]})})},V={render:()=>{let e=`Click ${F}here${I} to ${F}learn more${I}`;return(0,P.jsx)(`div`,{style:{maxWidth:640,padding:16},children:(0,P.jsxs)(M,{title:`Semantic Equivalence — HTML vs Markdown`,children:[(0,P.jsx)(`p`,{style:{fontSize:12,color:`#999`,marginBottom:16},children:`Same sentence with HTML tags vs Markdown delimiters. Both produce identical visual rendering because they map to the same semantic categories (bold, italic).`}),(0,P.jsxs)(`div`,{style:{display:`flex`,gap:32},children:[(0,P.jsxs)(`div`,{style:{flex:1},children:[(0,P.jsxs)(`div`,{style:{fontSize:10,color:`#888`,fontWeight:600,marginBottom:4},children:[`HTML (`,`<b>, <i>`,`)`]}),(0,P.jsx)(`div`,{style:{fontSize:14,lineHeight:1.6},children:(0,P.jsx)(a,{codedText:e,spans:[{span_type:`opening`,type:`b`,id:`1`,data:`<b>`},{span_type:`closing`,type:`b`,id:`1`,data:`</b>`},{span_type:`opening`,type:`i`,id:`2`,data:`<i>`},{span_type:`closing`,type:`i`,id:`2`,data:`</i>`}]})})]}),(0,P.jsxs)(`div`,{style:{flex:1},children:[(0,P.jsx)(`div`,{style:{fontSize:10,color:`#888`,fontWeight:600,marginBottom:4},children:`Markdown (**, *)`}),(0,P.jsx)(`div`,{style:{fontSize:14,lineHeight:1.6},children:(0,P.jsx)(a,{codedText:e,spans:[{span_type:`opening`,type:`bold`,id:`1`,data:`**`},{span_type:`closing`,type:`bold`,id:`1`,data:`**`},{span_type:`opening`,type:`italic`,id:`2`,data:`*`},{span_type:`closing`,type:`italic`,id:`2`,data:`*`}]})})]})]})]})})}},H={render:()=>(0,P.jsx)(`div`,{style:{maxWidth:640,padding:16},children:(0,P.jsxs)(M,{title:`Code View — Tag Chips (Opt-in)`,children:[(0,P.jsx)(`p`,{style:{fontSize:12,color:`#999`,marginBottom:12},children:`The same content shown with tag chip rendering. This is the advanced view that translators can toggle to when they need to see the raw inline code structure.`}),(0,P.jsxs)(`div`,{style:{marginBottom:12},children:[(0,P.jsx)(`div`,{style:{fontSize:10,color:`#888`,fontWeight:600,marginBottom:4},children:`HTML Bold + Link`}),(0,P.jsx)(`div`,{style:{fontSize:14,lineHeight:1.6},children:(0,P.jsx)(s,{codedText:_,spans:f})})]}),(0,P.jsxs)(`div`,{children:[(0,P.jsx)(`div`,{style:{fontSize:10,color:`#888`,fontWeight:600,marginBottom:4},children:`Markdown Code + Link`}),(0,P.jsx)(`div`,{style:{fontSize:14,lineHeight:1.6},children:(0,P.jsx)(s,{codedText:g,spans:d})})]})]})})},U={render:()=>(0,P.jsx)(`div`,{style:{maxWidth:640,padding:16},children:(0,P.jsx)(M,{title:`All Semantic Categories`,children:[{label:`Bold`,codedText:`This is ${F}bold text${I}`,spans:[{span_type:`opening`,type:`bold`,id:`1`,data:`**`},{span_type:`closing`,type:`bold`,id:`1`,data:`**`}]},{label:`Italic`,codedText:`This is ${F}italic text${I}`,spans:[{span_type:`opening`,type:`italic`,id:`2`,data:`*`},{span_type:`closing`,type:`italic`,id:`2`,data:`*`}]},{label:`Underline`,codedText:`This is ${F}underlined text${I}`,spans:[A,h]},{label:`Strikethrough`,codedText:`This is ${F}deleted text${I}`,spans:[v,m]},{label:`Link`,codedText:`Visit ${F}our website${I} for more`,spans:[{span_type:`opening`,type:`link`,id:`5`,data:`[`},{span_type:`closing`,type:`link`,id:`5`,data:`](https://example.com)`}]},{label:`Code`,codedText:`Run ${F}kapi init${I} to start`,spans:[{span_type:`opening`,type:`code`,id:`6`,data:"`"},{span_type:`closing`,type:`code`,id:`6`,data:"`"}]},{label:`Superscript`,codedText:`E=mc${F}2${I}`,spans:[p,b]},{label:`Line Break (placeholder)`,codedText:`First line${L}Second line`,spans:[{span_type:`placeholder`,type:`br`,id:`9`,data:`<br/>`}]},{label:`Image (placeholder)`,codedText:`See the logo ${L} below`,spans:[w]}].map(e=>(0,P.jsxs)(`div`,{style:{marginBottom:12},children:[(0,P.jsx)(`div`,{style:{fontSize:10,color:`#888`,fontWeight:600,marginBottom:4},children:e.label}),(0,P.jsx)(`div`,{style:{fontSize:14,lineHeight:1.6},children:(0,P.jsx)(a,{codedText:e.codedText,spans:e.spans})})]},e.label))})})},W={render:()=>{let e=[{span_type:`opening`,type:`fmt:bold`,id:`1`,data:`<b>`},{span_type:`closing`,type:`fmt:bold`,id:`1`,data:`</b>`},{span_type:`opening`,type:`link:hyperlink`,id:`2`,data:`<a href="https://example.com">`},{span_type:`closing`,type:`link:hyperlink`,id:`2`,data:`</a>`},{span_type:`opening`,type:`fmt:code`,id:`3`,data:`<code>`},{span_type:`closing`,type:`fmt:code`,id:`3`,data:`</code>`},{span_type:`placeholder`,type:`code:variable`,id:`4`,data:`{version}`,display_text:`{version}`,deletable:!1,cloneable:!1,can_reorder:!0},{span_type:`placeholder`,type:`struct:break`,id:`5`,data:`<br/>`}],t=`${F}Download${I} the latest ${F}release${I} (${F}v${I}${L}).${L}Visit the ${F}docs${I} for details.`;return(0,P.jsx)(`div`,{style:{maxWidth:640,padding:16},children:(0,P.jsxs)(M,{title:`Vocabulary-Driven Experience`,children:[(0,P.jsx)(`p`,{style:{fontSize:12,color:`#999`,marginBottom:16},children:`The complete translator workflow: vocabulary badge summarizes tag categories at a glance, the formatted view shows text naturally, and the legend explains each tag type with its constraints.`}),(0,P.jsxs)(`div`,{style:{marginBottom:12},children:[(0,P.jsx)(`div`,{style:{fontSize:10,color:`#888`,fontWeight:600,marginBottom:4},children:`Vocabulary Badge`}),(0,P.jsx)(l,{spans:e})]}),(0,P.jsxs)(`div`,{style:{marginBottom:12},children:[(0,P.jsx)(`div`,{style:{fontSize:10,color:`#888`,fontWeight:600,marginBottom:4},children:`Formatted View`}),(0,P.jsx)(`div`,{style:{fontSize:14,lineHeight:1.6},children:(0,P.jsx)(a,{codedText:t,spans:e})})]}),(0,P.jsxs)(`div`,{style:{marginBottom:12},children:[(0,P.jsx)(`div`,{style:{fontSize:10,color:`#888`,fontWeight:600,marginBottom:4},children:`Code View`}),(0,P.jsx)(`div`,{style:{fontSize:14,lineHeight:1.6},children:(0,P.jsx)(s,{codedText:t,spans:e})})]}),(0,P.jsxs)(`div`,{children:[(0,P.jsx)(`div`,{style:{fontSize:10,color:`#888`,fontWeight:600,marginBottom:4},children:`Inline Code Legend`}),(0,P.jsx)(n,{spans:e,onClose:()=>{}})]})]})})}},z.parameters={...z.parameters,docs:{...z.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    maxWidth: 640,
    padding: 16
  }}>
      <Section title="HTML Spans — Formatted View">
        <Comparison label="Bold" codedText={simpleBoldCodedText} spans={simpleBoldSpans} />
        <Comparison label="Link + Italic" codedText={linkAndItalicCodedText} spans={linkAndItalicSpans} />
        <Comparison label="Inline Code" codedText={codeInlineCodedText} spans={codeInlineSpans} />
        <Comparison label="Line Break" codedText={lineBreakCodedText} spans={lineBreakSpans} />
      </Section>
    </div>
}`,...z.parameters?.docs?.source},description:{story:`HTML inline codes rendered as formatted text with background tints.`,...z.parameters?.docs?.description}}},B.parameters={...B.parameters,docs:{...B.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    maxWidth: 640,
    padding: 16
  }}>
      <Section title="Markdown Spans — Formatted View">
        <Comparison label="Bold + Italic (** and *)" codedText={mdFormattingCodedText} spans={mdFormattingSpans} />
        <Comparison label="Code + Link (\` and []())" codedText={mdRichCodedText} spans={mdRichSpans} />
      </Section>
    </div>
}`,...B.parameters?.docs?.source},description:{story:`Markdown inline codes rendered as formatted text — same visual output
as HTML despite different underlying syntax.`,...B.parameters?.docs?.description}}},V.parameters={...V.parameters,docs:{...V.parameters?.docs,source:{originalSource:`{
  render: () => {
    // Same sentence, different underlying markup
    const sentence = \`Click \${O}here\${C} to \${O}learn more\${C}\`;
    const htmlSpans: SpanInfo[] = [{
      span_type: "opening",
      type: "b",
      id: "1",
      data: "<b>"
    }, {
      span_type: "closing",
      type: "b",
      id: "1",
      data: "</b>"
    }, {
      span_type: "opening",
      type: "i",
      id: "2",
      data: "<i>"
    }, {
      span_type: "closing",
      type: "i",
      id: "2",
      data: "</i>"
    }];
    const mdSpans: SpanInfo[] = [{
      span_type: "opening",
      type: "bold",
      id: "1",
      data: "**"
    }, {
      span_type: "closing",
      type: "bold",
      id: "1",
      data: "**"
    }, {
      span_type: "opening",
      type: "italic",
      id: "2",
      data: "*"
    }, {
      span_type: "closing",
      type: "italic",
      id: "2",
      data: "*"
    }];
    return <div style={{
      maxWidth: 640,
      padding: 16
    }}>
        <Section title="Semantic Equivalence — HTML vs Markdown">
          <p style={{
          fontSize: 12,
          color: "#999",
          marginBottom: 16
        }}>
            Same sentence with HTML tags vs Markdown delimiters. Both produce identical visual
            rendering because they map to the same semantic categories (bold, italic).
          </p>
          <div style={{
          display: "flex",
          gap: 32
        }}>
            <div style={{
            flex: 1
          }}>
              <div style={{
              fontSize: 10,
              color: "#888",
              fontWeight: 600,
              marginBottom: 4
            }}>
                HTML ({\`<b>, <i>\`})
              </div>
              <div style={{
              fontSize: 14,
              lineHeight: 1.6
            }}>
                <FormattedSourceDisplay codedText={sentence} spans={htmlSpans} />
              </div>
            </div>
            <div style={{
            flex: 1
          }}>
              <div style={{
              fontSize: 10,
              color: "#888",
              fontWeight: 600,
              marginBottom: 4
            }}>
                Markdown (**, *)
              </div>
              <div style={{
              fontSize: 14,
              lineHeight: 1.6
            }}>
                <FormattedSourceDisplay codedText={sentence} spans={mdSpans} />
              </div>
            </div>
          </div>
        </Section>
      </div>;
  }
}`,...V.parameters?.docs?.source},description:{story:`Side-by-side comparison showing that HTML and Markdown spans produce
identical visual output when rendered through FormattedSourceDisplay.`,...V.parameters?.docs?.description}}},H.parameters={...H.parameters,docs:{...H.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    maxWidth: 640,
    padding: 16
  }}>
      <Section title="Code View — Tag Chips (Opt-in)">
        <p style={{
        fontSize: 12,
        color: "#999",
        marginBottom: 12
      }}>
          The same content shown with tag chip rendering. This is the advanced view that translators
          can toggle to when they need to see the raw inline code structure.
        </p>
        <div style={{
        marginBottom: 12
      }}>
          <div style={{
          fontSize: 10,
          color: "#888",
          fontWeight: 600,
          marginBottom: 4
        }}>
            HTML Bold + Link
          </div>
          <div style={{
          fontSize: 14,
          lineHeight: 1.6
        }}>
            <SourceCellDisplay codedText={richCodedText} spans={richSpans} />
          </div>
        </div>
        <div>
          <div style={{
          fontSize: 10,
          color: "#888",
          fontWeight: 600,
          marginBottom: 4
        }}>
            Markdown Code + Link
          </div>
          <div style={{
          fontSize: 14,
          lineHeight: 1.6
        }}>
            <SourceCellDisplay codedText={mdRichCodedText} spans={mdRichSpans} />
          </div>
        </div>
      </Section>
    </div>
}`,...H.parameters?.docs?.source},description:{story:`The same segments shown with tag chips — the opt-in code view
for advanced users who need to see raw inline code structure.`,...H.parameters?.docs?.description}}},U.parameters={...U.parameters,docs:{...U.parameters?.docs,source:{originalSource:`{
  render: () => {
    const categories: Array<{
      label: string;
      codedText: string;
      spans: SpanInfo[];
    }> = [{
      label: "Bold",
      codedText: \`This is \${O}bold text\${C}\`,
      spans: [{
        span_type: "opening",
        type: "bold",
        id: "1",
        data: "**"
      }, {
        span_type: "closing",
        type: "bold",
        id: "1",
        data: "**"
      }]
    }, {
      label: "Italic",
      codedText: \`This is \${O}italic text\${C}\`,
      spans: [{
        span_type: "opening",
        type: "italic",
        id: "2",
        data: "*"
      }, {
        span_type: "closing",
        type: "italic",
        id: "2",
        data: "*"
      }]
    }, {
      label: "Underline",
      codedText: \`This is \${O}underlined text\${C}\`,
      spans: [underlineOpen, underlineClose]
    }, {
      label: "Strikethrough",
      codedText: \`This is \${O}deleted text\${C}\`,
      spans: [strikeOpen, strikeClose]
    }, {
      label: "Link",
      codedText: \`Visit \${O}our website\${C} for more\`,
      spans: [{
        span_type: "opening",
        type: "link",
        id: "5",
        data: "["
      }, {
        span_type: "closing",
        type: "link",
        id: "5",
        data: "](https://example.com)"
      }]
    }, {
      label: "Code",
      codedText: \`Run \${O}kapi init\${C} to start\`,
      spans: [{
        span_type: "opening",
        type: "code",
        id: "6",
        data: "\`"
      }, {
        span_type: "closing",
        type: "code",
        id: "6",
        data: "\`"
      }]
    }, {
      label: "Superscript",
      codedText: \`E=mc\${O}2\${C}\`,
      spans: [supOpen, supClose]
    }, {
      label: "Line Break (placeholder)",
      codedText: \`First line\${P}Second line\`,
      spans: [{
        span_type: "placeholder" as const,
        type: "br",
        id: "9",
        data: "<br/>"
      }]
    }, {
      label: "Image (placeholder)",
      codedText: \`See the logo \${P} below\`,
      spans: [imgTag]
    }];
    return <div style={{
      maxWidth: 640,
      padding: 16
    }}>
        <Section title="All Semantic Categories">
          {categories.map(cat => <div key={cat.label} style={{
          marginBottom: 12
        }}>
              <div style={{
            fontSize: 10,
            color: "#888",
            fontWeight: 600,
            marginBottom: 4
          }}>
                {cat.label}
              </div>
              <div style={{
            fontSize: 14,
            lineHeight: 1.6
          }}>
                <FormattedSourceDisplay codedText={cat.codedText} spans={cat.spans} />
              </div>
            </div>)}
        </Section>
      </div>;
  }
}`,...U.parameters?.docs?.source},description:{story:`One example per semantic category showing the formatted rendering.`,...U.parameters?.docs?.description}}},W.parameters={...W.parameters,docs:{...W.parameters?.docs,source:{originalSource:`{
  render: () => {
    const allSpans: SpanInfo[] = [{
      span_type: "opening",
      type: "fmt:bold",
      id: "1",
      data: "<b>"
    }, {
      span_type: "closing",
      type: "fmt:bold",
      id: "1",
      data: "</b>"
    }, {
      span_type: "opening",
      type: "link:hyperlink",
      id: "2",
      data: '<a href="https://example.com">'
    }, {
      span_type: "closing",
      type: "link:hyperlink",
      id: "2",
      data: "</a>"
    }, {
      span_type: "opening",
      type: "fmt:code",
      id: "3",
      data: "<code>"
    }, {
      span_type: "closing",
      type: "fmt:code",
      id: "3",
      data: "</code>"
    }, {
      span_type: "placeholder",
      type: "code:variable",
      id: "4",
      data: "{version}",
      display_text: "{version}",
      deletable: false,
      cloneable: false,
      can_reorder: true
    }, {
      span_type: "placeholder",
      type: "struct:break",
      id: "5",
      data: "<br/>"
    }];
    const codedText = \`\${O}Download\${C} the latest \${O}release\${C} (\${O}v\${C}\${P}).\${P}Visit the \${O}docs\${C} for details.\`;
    return <div style={{
      maxWidth: 640,
      padding: 16
    }}>
        <Section title="Vocabulary-Driven Experience">
          <p style={{
          fontSize: 12,
          color: "#999",
          marginBottom: 16
        }}>
            The complete translator workflow: vocabulary badge summarizes tag categories at a
            glance, the formatted view shows text naturally, and the legend explains each tag type
            with its constraints.
          </p>

          {/* Badge */}
          <div style={{
          marginBottom: 12
        }}>
            <div style={{
            fontSize: 10,
            color: "#888",
            fontWeight: 600,
            marginBottom: 4
          }}>
              Vocabulary Badge
            </div>
            <FormatVocabularyBadge spans={allSpans} />
          </div>

          {/* Formatted view */}
          <div style={{
          marginBottom: 12
        }}>
            <div style={{
            fontSize: 10,
            color: "#888",
            fontWeight: 600,
            marginBottom: 4
          }}>
              Formatted View
            </div>
            <div style={{
            fontSize: 14,
            lineHeight: 1.6
          }}>
              <FormattedSourceDisplay codedText={codedText} spans={allSpans} />
            </div>
          </div>

          {/* Code view */}
          <div style={{
          marginBottom: 12
        }}>
            <div style={{
            fontSize: 10,
            color: "#888",
            fontWeight: 600,
            marginBottom: 4
          }}>
              Code View
            </div>
            <div style={{
            fontSize: 14,
            lineHeight: 1.6
          }}>
              <SourceCellDisplay codedText={codedText} spans={allSpans} />
            </div>
          </div>

          {/* Legend */}
          <div>
            <div style={{
            fontSize: 10,
            color: "#888",
            fontWeight: 600,
            marginBottom: 4
          }}>
              Inline Code Legend
            </div>
            <InlineCodeLegend spans={allSpans} onClose={() => {}} />
          </div>
        </Section>
      </div>;
  }
}`,...W.parameters?.docs?.source},description:{story:`Full vocabulary-driven experience showing badge, legend, and formatted
rendering working together — the complete translator workflow.`,...W.parameters?.docs?.description}}},G=[`HTMLFormat`,`MarkdownFormat`,`SemanticEquivalence`,`CodeView`,`AllCategories`,`VocabularyDrivenExperience`]}))();export{U as AllCategories,H as CodeView,z as HTMLFormat,B as MarkdownFormat,V as SemanticEquivalence,W as VocabularyDrivenExperience,G as __namedExportsOrder,R as default};