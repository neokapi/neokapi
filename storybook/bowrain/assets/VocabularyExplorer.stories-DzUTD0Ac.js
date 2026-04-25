import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{t as i}from"./utils-BUNg-TRL.js";import{n as a,t as o}from"./src-Bvgh6dl3.js";import{Rt as s}from"./lucide-react-CMMmtkYs.js";import{o as c}from"./CodedTextDisplay-DKJcc4pM.js";import{t as l}from"./icons-CfKm4CHh.js";import{n as u,t as d}from"./vocabularies-DeCNtscR.js";import{n as f,t as p}from"./FormatVocabularyBadge-D2V_Ej-5.js";import{E as m,G as h,H as g,K as _,U as v,W as y,c as b,d as x,l as S,n as C,o as w,p as T,q as E,r as D,t as O,u as k,v as A,y as j}from"./fixtures-D8OXoGLB.js";function M(e){let t=[];return e.constraints.deletable||t.push({label:`Required`,color:`text-destructive bg-destructive/10 border-destructive/25`,description:`Must be present in the translation`}),e.constraints.cloneable||t.push({label:`No duplicates`,color:`text-warning bg-warning/10 border-warning/25`,description:`Cannot be duplicated in the translation`}),e.constraints.reorderable||t.push({label:`Fixed position`,color:`text-purple-500 bg-purple-500/10 border-purple-500/25`,description:`Must stay in the same relative position`}),t}function N(e,t,n){return{span_type:t,type:e,id:n,data:``}}function P({activeTypes:e,compact:t,onTypeSelect:n}){let r=d(),[a,o]=(0,I.useState)(null),l=(0,I.useMemo)(()=>r.categories().map(e=>{let t=r.typesInCategory(e);return{category:e,label:R[e]||e,types:t.map(e=>({name:e,info:r.lookupOrFallback(e)}))}}),[r]),u=e=>{o(t=>t===e?null:e)};return(0,L.jsx)(`div`,{className:`flex flex-col gap-1`,children:l.map(r=>{let o=a===r.category;return(0,L.jsxs)(`div`,{className:i(`rounded-md border border-border/50 overflow-hidden transition-all`,!(!e||r.types.some(t=>e.includes(t.name)))&&e&&`opacity-40`),children:[(0,L.jsxs)(`button`,{type:`button`,onClick:()=>u(r.category),className:`w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-muted/40 transition-colors`,children:[(0,L.jsx)(F,{color:r.types[0]?.info.color}),(0,L.jsxs)(`div`,{className:`flex-1 min-w-0`,children:[(0,L.jsx)(`div`,{className:`text-xs font-semibold text-foreground`,children:r.label}),!t&&(0,L.jsxs)(`div`,{className:`text-[10px] text-muted-foreground leading-tight`,children:[r.types.length,` type`,r.types.length===1?``:`s`]})]}),!o&&(0,L.jsxs)(`div`,{className:`flex items-center gap-0.5 mr-2`,children:[r.types.slice(0,4).map(t=>(0,L.jsx)(c,{spanInfo:N(t.name,t.info.chipLabel.placeholder?`placeholder`:`opening`,`p`),dimmed:e?!e.includes(t.name):!1},t.name)),r.types.length>4&&(0,L.jsxs)(`span`,{className:`text-[10px] text-muted-foreground ml-1`,children:[`+`,r.types.length-4]})]}),(0,L.jsx)(s,{className:i(`w-3.5 h-3.5 text-muted-foreground transition-transform shrink-0`,o&&`rotate-180`)})]}),o&&(0,L.jsxs)(`div`,{className:`border-t border-border/30`,children:[!t&&z[r.category]&&(0,L.jsx)(`div`,{className:`px-3 py-1.5 text-[11px] text-muted-foreground bg-muted/20 border-b border-border/20`,children:z[r.category]}),r.types.map(r=>{let a=!e||e.includes(r.name),o=M(r.info);return(0,L.jsxs)(`div`,{className:i(`flex items-start gap-3 px-3 py-2 border-b border-border/20 last:border-b-0`,`hover:bg-muted/30 transition-colors`,n&&`cursor-pointer`,!a&&`opacity-40`),onClick:()=>n?.(r.name),children:[(0,L.jsxs)(`div`,{className:`flex items-center gap-1 pt-0.5 shrink-0`,children:[r.info.chipLabel.open&&(0,L.jsx)(c,{spanInfo:N(r.name,`opening`,`x`)}),r.info.chipLabel.close&&(0,L.jsx)(c,{spanInfo:N(r.name,`closing`,`x`)}),r.info.chipLabel.placeholder&&!r.info.chipLabel.open&&(0,L.jsx)(c,{spanInfo:N(r.name,`placeholder`,`x`)})]}),(0,L.jsxs)(`div`,{className:`flex-1 min-w-0`,children:[(0,L.jsxs)(`div`,{className:`flex items-center gap-2`,children:[(0,L.jsx)(`span`,{className:`text-xs font-medium text-foreground`,children:r.info.label}),(0,L.jsx)(`span`,{className:`text-[10px] text-muted-foreground font-mono`,children:r.name})]}),o.length>0&&(0,L.jsx)(`div`,{className:`flex items-center gap-1 mt-1`,children:o.map(e=>(0,L.jsx)(`span`,{className:i(`text-[9px] px-1.5 py-0.5 rounded border font-medium`,e.color),title:e.description,children:e.label},e.label))})]}),!t&&(0,L.jsx)(`div`,{className:`text-xs text-muted-foreground shrink-0 pt-0.5 font-mono`,children:r.info.html.open&&r.info.html.close?`${r.info.html.open}…${r.info.html.close}`:r.info.html.placeholder||``})]},r.name)})]})]},r.category)})})}function F({color:e}){return(0,L.jsx)(`div`,{className:`w-2.5 h-2.5 rounded-full shrink-0`,style:{backgroundColor:e?.text||`var(--text-muted)`,opacity:.8}})}var I,L,R,z,B=t((()=>{I=e(n(),1),u(),o(),l(),L=r(),R={formatting:`Text Formatting`,linking:`Links & References`,media:`Images & Media`,structure:`Document Structure`,code:`Code & Variables`,generic:`Other`},z={formatting:`Visual text styles like bold, italic, and underline that should be preserved in translations.`,linking:`Hyperlinks and cross-references — the linked text is translated but the URL is preserved.`,media:`Embedded images, videos, and other media that appear inline within text.`,structure:`Structural elements like line breaks and footnotes that control document layout.`,code:`Variables, placeholders, and function calls that must not be modified during translation.`},P.__docgenInfo={description:``,methods:[],displayName:`VocabularyExplorer`,props:{activeTypes:{required:!1,tsType:{name:`Array`,elements:[{name:`string`}],raw:`string[]`},description:`Highlight only types present in this array (filter mode).`},compact:{required:!1,tsType:{name:`boolean`},description:`Compact mode hides descriptions and shows fewer details.`},onTypeSelect:{required:!1,tsType:{name:`signature`,type:`function`,raw:`(typeName: string) => void`,signature:{arguments:[{type:{name:`string`},name:`typeName`}],return:{name:`void`}}},description:`Called when user clicks a type entry.`}}}})),V,H,U,W,G,K,q,J,Y,X,Z,Q,$;t((()=>{B(),o(),f(),S(),V=r(),H={title:`Editor/Terminology/VocabularyExplorer`,tags:[`autodocs`],parameters:{docs:{description:{component:[`Interactive explorer for the vocabulary system that drives inline code rendering.`,``,`Vocabularies define how each inline code type (bold, links, variables, etc.) looks,`,`behaves, and is constrained in the translation editor. The same vocabulary drives`,`consistent behavior across HTML, Markdown, XLIFF, and all other formats.`,``,`**Key features:**`,`- Category-grouped type browser with chip previews`,`- Constraint indicators (required, no duplicates, fixed position)`,"- Format-agnostic — HTML `<b>` and Markdown `**` both map to `fmt:bold`"].join(`
`)}}}},U={render:()=>(0,V.jsxs)(`div`,{style:{maxWidth:480,padding:16},children:[(0,V.jsx)(`h3`,{style:X,children:`Vocabulary Explorer`}),(0,V.jsx)(`p`,{style:Z,children:`Click a category to expand it and see all span types, their chip representations, and editing constraints.`}),(0,V.jsx)(P,{})]})},W={render:()=>(0,V.jsxs)(`div`,{style:{maxWidth:480,padding:16},children:[(0,V.jsx)(`h3`,{style:X,children:`Filtered by Document Types`}),(0,V.jsx)(`p`,{style:Z,children:`When a document only uses certain types, inactive categories are dimmed. This document uses bold, italic, and hyperlinks.`}),(0,V.jsx)(P,{activeTypes:[`fmt:bold`,`fmt:italic`,`link:hyperlink`]})]})},G={render:()=>(0,V.jsxs)(`div`,{style:{maxWidth:320,padding:16},children:[(0,V.jsx)(`h3`,{style:X,children:`Compact Mode`}),(0,V.jsx)(P,{compact:!0})]})},K={render:()=>(0,V.jsxs)(`div`,{style:{maxWidth:400,padding:16},children:[(0,V.jsx)(`h3`,{style:X,children:`Inline Code Legend`}),(0,V.jsx)(`p`,{style:Z,children:`Shows all inline tag types in the current segment, grouped by category, with constraint indicators.`}),(0,V.jsx)(a,{spans:[C,O,j,A,w,D,T],onClose:()=>{}})]})},q={render:()=>(0,V.jsxs)(`div`,{style:{maxWidth:400,padding:16},children:[(0,V.jsx)(`h3`,{style:X,children:`Code Tokens + Formatting`}),(0,V.jsx)(`p`,{style:Z,children:`A segment mixing code tokens (variables, placeholders, ICU functions) with formatting tags. Note the different constraint levels.`}),(0,V.jsx)(a,{spans:[{span_type:`placeholder`,type:`code:variable`,id:`1`,data:`{userName}`,deletable:!1,cloneable:!1,can_reorder:!0},{span_type:`placeholder`,type:`code:placeholder`,id:`2`,data:`{0}`,deletable:!1,cloneable:!1,can_reorder:!0},{span_type:`opening`,type:`code:function`,id:`3`,data:`{count, plural,`,deletable:!1,cloneable:!1,can_reorder:!1},{span_type:`closing`,type:`code:function`,id:`3`,data:`}`,deletable:!1,cloneable:!1,can_reorder:!1},C,O],onClose:()=>{}})]})},J={render:()=>(0,V.jsxs)(`div`,{style:{maxWidth:480,padding:16},children:[(0,V.jsx)(`h3`,{style:X,children:`Vocabulary Badge`}),(0,V.jsx)(`p`,{style:Z,children:`Compact inline badge showing which vocabulary categories are active. Typically displayed in the editor card header.`}),(0,V.jsxs)(`div`,{style:{display:`flex`,flexDirection:`column`,gap:12,marginTop:16},children:[(0,V.jsxs)(`div`,{children:[(0,V.jsx)(`span`,{style:Q,children:`Simple formatting:`}),(0,V.jsx)(p,{spans:[C,O,x,k]})]}),(0,V.jsxs)(`div`,{children:[(0,V.jsx)(`span`,{style:Q,children:`Rich content:`}),(0,V.jsx)(p,{spans:m})]}),(0,V.jsxs)(`div`,{children:[(0,V.jsx)(`span`,{style:Q,children:`Code tokens:`}),(0,V.jsx)(p,{spans:[{span_type:`placeholder`,type:`code:variable`,id:`1`,data:`{name}`},{span_type:`placeholder`,type:`code:placeholder`,id:`2`,data:`{0}`},C,O]})]})]})]})},Y={render:()=>(0,V.jsxs)(`div`,{style:{maxWidth:640,padding:16},children:[(0,V.jsx)(`h3`,{style:X,children:`All Chip Styles`}),(0,V.jsx)(`p`,{style:Z,children:`Every vocabulary type rendered as tag chips, with constraint indicators.`}),(0,V.jsx)(`div`,{style:{display:`flex`,flexDirection:`column`,gap:8,marginTop:16},children:[{label:`Bold`,spans:[C,O]},{label:`Italic`,spans:[x,k]},{label:`Underline`,spans:[E,_]},{label:`Strikethrough`,spans:[v,g]},{label:`Superscript`,spans:[h,y]},{label:`Hyperlink`,spans:[j,A]},{label:`Inline Code`,spans:[w,D]},{label:`Line Break`,spans:[T]},{label:`Image`,spans:[b]},{label:`Variable`,spans:[{span_type:`placeholder`,type:`code:variable`,id:`1`,data:`{name}`}]},{label:`Placeholder`,spans:[{span_type:`placeholder`,type:`code:placeholder`,id:`1`,data:`{0}`}]},{label:`Function (ICU)`,spans:[{span_type:`opening`,type:`code:function`,id:`1`,data:`{count, plural,`},{span_type:`closing`,type:`code:function`,id:`1`,data:`}`}]}].map(({label:e,spans:t})=>(0,V.jsxs)(`div`,{style:{display:`flex`,alignItems:`center`,gap:8},children:[(0,V.jsx)(`span`,{style:{...Q,width:120},children:e}),(0,V.jsx)(`div`,{style:{display:`flex`,gap:4},children:t.map((e,t)=>(0,V.jsx)(c,{spanInfo:e,showConstraints:!0},t))})]},e))})]})},X={fontSize:14,fontWeight:600,marginBottom:4},Z={fontSize:12,color:`#888`,marginBottom:16,lineHeight:1.5},Q={fontSize:12,color:`#888`,marginRight:8,display:`inline-block`},U.parameters={...U.parameters,docs:{...U.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    maxWidth: 480,
    padding: 16
  }}>
      <h3 style={titleStyle}>Vocabulary Explorer</h3>
      <p style={descStyle}>
        Click a category to expand it and see all span types, their chip representations, and
        editing constraints.
      </p>
      <VocabularyExplorer />
    </div>
}`,...U.parameters?.docs?.source},description:{story:`Browse all vocabulary categories and their span types.`,...U.parameters?.docs?.description}}},W.parameters={...W.parameters,docs:{...W.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    maxWidth: 480,
    padding: 16
  }}>
      <h3 style={titleStyle}>Filtered by Document Types</h3>
      <p style={descStyle}>
        When a document only uses certain types, inactive categories are dimmed. This document uses
        bold, italic, and hyperlinks.
      </p>
      <VocabularyExplorer activeTypes={["fmt:bold", "fmt:italic", "link:hyperlink"]} />
    </div>
}`,...W.parameters?.docs?.source},description:{story:`Filter mode: only types present in the current document are highlighted.`,...W.parameters?.docs?.description}}},G.parameters={...G.parameters,docs:{...G.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    maxWidth: 320,
    padding: 16
  }}>
      <h3 style={titleStyle}>Compact Mode</h3>
      <VocabularyExplorer compact />
    </div>
}`,...G.parameters?.docs?.source},description:{story:`Compact mode for tighter spaces like sidebars.`,...G.parameters?.docs?.description}}},K.parameters={...K.parameters,docs:{...K.parameters?.docs,source:{originalSource:`{
  render: () => {
    const spans: SpanInfo[] = [boldOpen, boldClose, linkOpen, linkClose, codeOpen, codeClose, lineBreak];
    return <div style={{
      maxWidth: 400,
      padding: 16
    }}>
        <h3 style={titleStyle}>Inline Code Legend</h3>
        <p style={descStyle}>
          Shows all inline tag types in the current segment, grouped by category, with constraint
          indicators.
        </p>
        <InlineCodeLegend spans={spans} onClose={() => {}} />
      </div>;
  }
}`,...K.parameters?.docs?.source},description:{story:`Inline code legend showing tags in the current segment.`,...K.parameters?.docs?.description}}},q.parameters={...q.parameters,docs:{...q.parameters?.docs,source:{originalSource:`{
  render: () => {
    const spans: SpanInfo[] = [{
      span_type: "placeholder",
      type: "code:variable",
      id: "1",
      data: "{userName}",
      deletable: false,
      cloneable: false,
      can_reorder: true
    }, {
      span_type: "placeholder",
      type: "code:placeholder",
      id: "2",
      data: "{0}",
      deletable: false,
      cloneable: false,
      can_reorder: true
    }, {
      span_type: "opening",
      type: "code:function",
      id: "3",
      data: "{count, plural,",
      deletable: false,
      cloneable: false,
      can_reorder: false
    }, {
      span_type: "closing",
      type: "code:function",
      id: "3",
      data: "}",
      deletable: false,
      cloneable: false,
      can_reorder: false
    }, boldOpen, boldClose];
    return <div style={{
      maxWidth: 400,
      padding: 16
    }}>
        <h3 style={titleStyle}>Code Tokens + Formatting</h3>
        <p style={descStyle}>
          A segment mixing code tokens (variables, placeholders, ICU functions) with formatting
          tags. Note the different constraint levels.
        </p>
        <InlineCodeLegend spans={spans} onClose={() => {}} />
      </div>;
  }
}`,...q.parameters?.docs?.source},description:{story:`Legend for a segment with code tokens (variables, placeholders).`,...q.parameters?.docs?.description}}},J.parameters={...J.parameters,docs:{...J.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    maxWidth: 480,
    padding: 16
  }}>
      <h3 style={titleStyle}>Vocabulary Badge</h3>
      <p style={descStyle}>
        Compact inline badge showing which vocabulary categories are active. Typically displayed in
        the editor card header.
      </p>
      <div style={{
      display: "flex",
      flexDirection: "column",
      gap: 12,
      marginTop: 16
    }}>
        <div>
          <span style={labelStyle}>Simple formatting:</span>
          <FormatVocabularyBadge spans={[boldOpen, boldClose, italicOpen, italicClose]} />
        </div>
        <div>
          <span style={labelStyle}>Rich content:</span>
          <FormatVocabularyBadge spans={richSpans} />
        </div>
        <div>
          <span style={labelStyle}>Code tokens:</span>
          <FormatVocabularyBadge spans={[{
          span_type: "placeholder",
          type: "code:variable",
          id: "1",
          data: "{name}"
        }, {
          span_type: "placeholder",
          type: "code:placeholder",
          id: "2",
          data: "{0}"
        }, boldOpen, boldClose]} />
        </div>
      </div>
    </div>
}`,...J.parameters?.docs?.source},description:{story:`Format vocabulary badge — compact summary of tag categories.`,...J.parameters?.docs?.description}}},Y.parameters={...Y.parameters,docs:{...Y.parameters?.docs,source:{originalSource:`{
  render: () => {
    const allTypes: Array<{
      label: string;
      spans: SpanInfo[];
    }> = [{
      label: "Bold",
      spans: [boldOpen, boldClose]
    }, {
      label: "Italic",
      spans: [italicOpen, italicClose]
    }, {
      label: "Underline",
      spans: [underlineOpen, underlineClose]
    }, {
      label: "Strikethrough",
      spans: [strikeOpen, strikeClose]
    }, {
      label: "Superscript",
      spans: [supOpen, supClose]
    }, {
      label: "Hyperlink",
      spans: [linkOpen, linkClose]
    }, {
      label: "Inline Code",
      spans: [codeOpen, codeClose]
    }, {
      label: "Line Break",
      spans: [lineBreak]
    }, {
      label: "Image",
      spans: [imgTag]
    }, {
      label: "Variable",
      spans: [{
        span_type: "placeholder",
        type: "code:variable",
        id: "1",
        data: "{name}"
      }]
    }, {
      label: "Placeholder",
      spans: [{
        span_type: "placeholder",
        type: "code:placeholder",
        id: "1",
        data: "{0}"
      }]
    }, {
      label: "Function (ICU)",
      spans: [{
        span_type: "opening",
        type: "code:function",
        id: "1",
        data: "{count, plural,"
      }, {
        span_type: "closing",
        type: "code:function",
        id: "1",
        data: "}"
      }]
    }];
    return <div style={{
      maxWidth: 640,
      padding: 16
    }}>
        <h3 style={titleStyle}>All Chip Styles</h3>
        <p style={descStyle}>
          Every vocabulary type rendered as tag chips, with constraint indicators.
        </p>
        <div style={{
        display: "flex",
        flexDirection: "column",
        gap: 8,
        marginTop: 16
      }}>
          {allTypes.map(({
          label,
          spans
        }) => <div key={label} style={{
          display: "flex",
          alignItems: "center",
          gap: 8
        }}>
              <span style={{
            ...labelStyle,
            width: 120
          }}>{label}</span>
              <div style={{
            display: "flex",
            gap: 4
          }}>
                {spans.map((s, i) => <TagChipComponent key={i} spanInfo={s} showConstraints />)}
              </div>
            </div>)}
        </div>
      </div>;
  }
}`,...Y.parameters?.docs?.source},description:{story:`All chip styles across every vocabulary type.`,...Y.parameters?.docs?.description}}},$=[`AllVocabularies`,`FilteredByActiveTypes`,`CompactMode`,`InlineCodeLegendPanel`,`CodeTokenLegend`,`VocabularyBadge`,`AllChipStyles`]}))();export{Y as AllChipStyles,U as AllVocabularies,q as CodeTokenLegend,G as CompactMode,W as FilteredByActiveTypes,K as InlineCodeLegendPanel,J as VocabularyBadge,$ as __namedExportsOrder,H as default};