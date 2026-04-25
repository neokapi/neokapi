import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./src-Q72DuJW2.js";import{o as i}from"./CodedTextDisplay-DKJcc4pM.js";import{l as a,s as o}from"./TMGroupedEntry-CGOac2Ak.js";import{n as s,t as c}from"./FormattedSourceDisplay-IUO0m4SR.js";import{n as l,t as u}from"./SourceCellDisplay-C_5x1VBh.js";var d,f,p,m,h,g,_,v,y,b,x,S,C,w,T,E,D,O,k,A,j,M,N,P,F;e((()=>{r(),s(),l(),d=t(),f=``,p=``,m=``,h={title:`Editor/Formatting/ConstraintBehaviors`,tags:[`autodocs`],parameters:{docs:{description:{component:[`Demonstrates the three editing constraints defined by vocabularies.`,``,`Each inline code type declares constraints that control translator behavior:`,``,`| Constraint | What it controls | Example |`,`|---|---|---|`,`| **Deletable** | Can the tag be removed from the translation? | Line breaks and variables are non-deletable |`,`| **Cloneable** | Can the tag be duplicated in the translation? | Variables cannot be duplicated |`,`| **Reorderable** | Can the tag be moved relative to others? | ICU functions must stay in position |`,``,`Constraints are defined in the vocabulary but can be overridden per-span.`].join(`
`)}}}},g={span_type:`opening`,type:`fmt:bold`,id:`1`,data:`<b>`},_={span_type:`closing`,type:`fmt:bold`,id:`1`,data:`</b>`},v={span_type:`placeholder`,type:`struct:break`,id:`2`,data:`<br/>`,deletable:!1,cloneable:!1,can_reorder:!1},y={span_type:`placeholder`,type:`code:variable`,id:`3`,data:`{userName}`,display_text:`{userName}`,deletable:!1,cloneable:!1,can_reorder:!0},b={span_type:`placeholder`,type:`code:placeholder`,id:`4`,data:`{0}`,display_text:`{0}`,deletable:!1,cloneable:!1,can_reorder:!0},x={span_type:`opening`,type:`code:function`,id:`5`,data:`{count, plural,`,display_text:`plural(`,deletable:!1,cloneable:!1,can_reorder:!1},S={span_type:`closing`,type:`code:function`,id:`5`,data:`}`,display_text:`)`,deletable:!1,cloneable:!1,can_reorder:!1},C={render:()=>(0,d.jsxs)(`div`,{style:{maxWidth:640,padding:16},children:[(0,d.jsx)(`h3`,{style:D,children:`Constraint Levels`}),(0,d.jsx)(`p`,{style:O,children:`Vocabulary types define three orthogonal constraints. Tags display visual indicators based on their constraint level.`}),(0,d.jsxs)(`div`,{style:k,children:[(0,d.jsx)(`h4`,{style:A,children:`Fully Flexible`}),(0,d.jsx)(`p`,{style:j,children:`Formatting tags like bold and italic can be freely deleted, duplicated, and reordered. Translators have full control.`}),(0,d.jsxs)(`div`,{style:N,children:[(0,d.jsx)(i,{spanInfo:g,showConstraints:!0}),(0,d.jsx)(i,{spanInfo:_,showConstraints:!0}),(0,d.jsx)(`span`,{style:P,children:`No constraint indicators — full freedom`})]})]}),(0,d.jsxs)(`div`,{style:k,children:[(0,d.jsx)(`h4`,{style:A,children:`Required Tags (non-deletable)`}),(0,d.jsx)(`p`,{style:j,children:`Line breaks and variables must appear in the translation. The editor prevents accidental deletion and shows a dashed border.`}),(0,d.jsxs)(`div`,{style:N,children:[(0,d.jsx)(i,{spanInfo:v,showConstraints:!0,locked:!0}),(0,d.jsx)(i,{spanInfo:y,showConstraints:!0,locked:!0}),(0,d.jsx)(i,{spanInfo:b,showConstraints:!0,locked:!0}),(0,d.jsx)(`span`,{style:P,children:`Dashed border + * indicator`})]})]}),(0,d.jsxs)(`div`,{style:k,children:[(0,d.jsx)(`h4`,{style:A,children:`Fixed Position (non-reorderable)`}),(0,d.jsx)(`p`,{style:j,children:`ICU plural/select functions and structural elements must maintain their position relative to other tags.`}),(0,d.jsxs)(`div`,{style:N,children:[(0,d.jsx)(i,{spanInfo:x,showConstraints:!0,locked:!0}),(0,d.jsx)(i,{spanInfo:S,showConstraints:!0,locked:!0}),(0,d.jsx)(`span`,{style:P,children:`Non-deletable + non-cloneable + fixed position`})]})]})]})},w={render:()=>{let e=[g,_,y,b,v];return(0,d.jsxs)(`div`,{style:{maxWidth:640,padding:16},children:[(0,d.jsx)(`h3`,{style:D,children:`Palette Constraint Behavior`}),(0,d.jsx)(`p`,{style:O,children:`When tags are already used in the target, the palette dims them. Non-cloneable tags are blocked (cannot be inserted again), while cloneable tags like bold remain insertable (just dimmed).`}),(0,d.jsxs)(`div`,{style:{marginBottom:16},children:[(0,d.jsx)(`div`,{style:M,children:`Source tags:`}),(0,d.jsx)(a,{sourceSpans:e,onInsert:()=>{},showCategoryGroups:!0})]}),(0,d.jsxs)(`div`,{children:[(0,d.jsx)(`div`,{style:M,children:`After using bold + variable in target:`}),(0,d.jsx)(a,{sourceSpans:e,usedSpans:[g,_,y],onInsert:()=>{},showCategoryGroups:!0}),(0,d.jsx)(`p`,{style:{...j,marginTop:8},children:`Bold tags are dimmed but still insertable (cloneable). The variable tag is blocked — hovering shows "cannot be duplicated".`})]})]})}},T={render:()=>(0,d.jsxs)(`div`,{style:{maxWidth:640,padding:16},children:[(0,d.jsx)(`h3`,{style:D,children:`Constraint Violation Messages`}),(0,d.jsx)(`p`,{style:O,children:`The validation bar shows real-time feedback when constraints are violated. Non-deletable tags generate errors when missing. Non-cloneable tags generate errors when duplicated.`}),(0,d.jsxs)(`div`,{style:k,children:[(0,d.jsx)(`div`,{style:M,children:`All tags present (valid):`}),(0,d.jsx)(o,{validation:{valid:!0,errors:[],warnings:[]}}),(0,d.jsx)(`p`,{style:P,children:`No validation bar shown — clean state.`})]}),(0,d.jsxs)(`div`,{style:k,children:[(0,d.jsx)(`div`,{style:M,children:`Required tags missing:`}),(0,d.jsx)(o,{validation:{valid:!1,errors:[{type:`deleted_non_deletable`,message:`Missing 1 non-deletable placeholder "Variable" tag`},{type:`deleted_non_deletable`,message:`Missing 1 non-deletable placeholder "Line Break" tag`}],warnings:[]}})]}),(0,d.jsxs)(`div`,{style:k,children:[(0,d.jsx)(`div`,{style:M,children:`Non-cloneable tag duplicated:`}),(0,d.jsx)(o,{validation:{valid:!1,errors:[{type:`cloned_non_cloneable`,message:`Duplicated 1 non-cloneable placeholder "Variable" tag`}],warnings:[]}})]}),(0,d.jsxs)(`div`,{style:k,children:[(0,d.jsx)(`div`,{style:M,children:`Multiple issues:`}),(0,d.jsx)(o,{validation:{valid:!1,errors:[{type:`deleted_non_deletable`,message:`Missing 1 non-deletable placeholder "Placeholder" tag`},{type:`unpaired`,message:`Closing "Bold" without matching opening tag`}],warnings:[{type:`extra_tag`,message:`Extra 1 opening "Italic" tag`}]}})]})]})},E={render:()=>{let e=`Hello ${m}, you have ${m} new ${f}messages${p} in your ${f}inbox${p}.${m}Check them now.`,t=[{span_type:`placeholder`,type:`code:variable`,id:`1`,data:`{userName}`,display_text:`{userName}`,deletable:!1,cloneable:!1,can_reorder:!0},{span_type:`placeholder`,type:`code:placeholder`,id:`2`,data:`{count}`,display_text:`{count}`,deletable:!1,cloneable:!1,can_reorder:!0},{span_type:`opening`,type:`fmt:bold`,id:`3`,data:`<b>`},{span_type:`closing`,type:`fmt:bold`,id:`3`,data:`</b>`},{span_type:`opening`,type:`link:hyperlink`,id:`4`,data:`<a href="/inbox">`},{span_type:`closing`,type:`link:hyperlink`,id:`4`,data:`</a>`},{span_type:`placeholder`,type:`struct:break`,id:`5`,data:`<br/>`,deletable:!1,cloneable:!1,can_reorder:!1}];return(0,d.jsxs)(`div`,{style:{maxWidth:700,padding:16},children:[(0,d.jsx)(`h3`,{style:D,children:`Real-World: Notification Message`}),(0,d.jsx)(`p`,{style:O,children:`A typical notification string mixing variables, formatting, links, and a line break. The legend shows how each type is constrained.`}),(0,d.jsxs)(`div`,{style:{marginBottom:16},children:[(0,d.jsx)(`div`,{style:M,children:`Formatted view`}),(0,d.jsx)(`div`,{style:{fontSize:14,lineHeight:1.8},children:(0,d.jsx)(c,{codedText:e,spans:t})})]}),(0,d.jsxs)(`div`,{style:{marginBottom:16},children:[(0,d.jsx)(`div`,{style:M,children:`Code view`}),(0,d.jsx)(`div`,{style:{fontSize:14,lineHeight:1.8},children:(0,d.jsx)(u,{codedText:e,spans:t})})]}),(0,d.jsxs)(`div`,{style:{marginBottom:16},children:[(0,d.jsx)(`div`,{style:M,children:`Tag palette`}),(0,d.jsx)(a,{sourceSpans:t,onInsert:()=>{},showCategoryGroups:!0})]}),(0,d.jsx)(`div`,{children:(0,d.jsx)(n,{spans:t,onClose:()=>{}})})]})}},D={fontSize:14,fontWeight:600,marginBottom:4},O={fontSize:12,color:`#888`,marginBottom:16,lineHeight:1.5},k={marginBottom:20},A={fontSize:12,fontWeight:600,marginBottom:4},j={fontSize:11,color:`#888`,marginBottom:8,lineHeight:1.4},M={fontSize:10,fontWeight:600,color:`#888`,textTransform:`uppercase`,letterSpacing:`0.05em`,marginBottom:4},N={display:`flex`,alignItems:`center`,gap:6,flexWrap:`wrap`},P={fontSize:10,color:`#999`,fontStyle:`italic`,marginLeft:4},C.parameters={...C.parameters,docs:{...C.parameters?.docs,source:{originalSource:`{
  render: () => <div style={{
    maxWidth: 640,
    padding: 16
  }}>
      <h3 style={titleStyle}>Constraint Levels</h3>
      <p style={descStyle}>
        Vocabulary types define three orthogonal constraints. Tags display visual indicators based
        on their constraint level.
      </p>

      {/* Fully flexible */}
      <div style={sectionStyle}>
        <h4 style={sectionTitleStyle}>Fully Flexible</h4>
        <p style={sectionDescStyle}>
          Formatting tags like bold and italic can be freely deleted, duplicated, and reordered.
          Translators have full control.
        </p>
        <div style={chipRowStyle}>
          <TagChipComponent spanInfo={flexBoldOpen} showConstraints />
          <TagChipComponent spanInfo={flexBoldClose} showConstraints />
          <span style={hintStyle}>No constraint indicators — full freedom</span>
        </div>
      </div>

      {/* Required (non-deletable) */}
      <div style={sectionStyle}>
        <h4 style={sectionTitleStyle}>Required Tags (non-deletable)</h4>
        <p style={sectionDescStyle}>
          Line breaks and variables must appear in the translation. The editor prevents accidental
          deletion and shows a dashed border.
        </p>
        <div style={chipRowStyle}>
          <TagChipComponent spanInfo={requiredBreak} showConstraints locked />
          <TagChipComponent spanInfo={requiredVariable} showConstraints locked />
          <TagChipComponent spanInfo={requiredPlaceholder} showConstraints locked />
          <span style={hintStyle}>Dashed border + * indicator</span>
        </div>
      </div>

      {/* Fixed position (non-reorderable) */}
      <div style={sectionStyle}>
        <h4 style={sectionTitleStyle}>Fixed Position (non-reorderable)</h4>
        <p style={sectionDescStyle}>
          ICU plural/select functions and structural elements must maintain their position relative
          to other tags.
        </p>
        <div style={chipRowStyle}>
          <TagChipComponent spanInfo={fixedFunction} showConstraints locked />
          <TagChipComponent spanInfo={fixedFunctionClose} showConstraints locked />
          <span style={hintStyle}>Non-deletable + non-cloneable + fixed position</span>
        </div>
      </div>
    </div>
}`,...C.parameters?.docs?.source},description:{story:`Overview of all three constraint levels.`,...C.parameters?.docs?.description}}},w.parameters={...w.parameters,docs:{...w.parameters?.docs,source:{originalSource:`{
  render: () => {
    const sourceSpans: SpanInfo[] = [flexBoldOpen, flexBoldClose, requiredVariable, requiredPlaceholder, requiredBreak];

    // Simulate: bold and variable already used in target
    const usedSpans: SpanInfo[] = [flexBoldOpen, flexBoldClose, requiredVariable];
    return <div style={{
      maxWidth: 640,
      padding: 16
    }}>
        <h3 style={titleStyle}>Palette Constraint Behavior</h3>
        <p style={descStyle}>
          When tags are already used in the target, the palette dims them. Non-cloneable tags are
          blocked (cannot be inserted again), while cloneable tags like bold remain insertable (just
          dimmed).
        </p>
        <div style={{
        marginBottom: 16
      }}>
          <div style={sectionLabel}>Source tags:</div>
          <TagPalette sourceSpans={sourceSpans} onInsert={() => {}} showCategoryGroups />
        </div>
        <div>
          <div style={sectionLabel}>After using bold + variable in target:</div>
          <TagPalette sourceSpans={sourceSpans} usedSpans={usedSpans} onInsert={() => {}} showCategoryGroups />
          <p style={{
          ...sectionDescStyle,
          marginTop: 8
        }}>
            Bold tags are dimmed but still insertable (cloneable). The variable tag is blocked —
            hovering shows "cannot be duplicated".
          </p>
        </div>
      </div>;
  }
}`,...w.parameters?.docs?.source},description:{story:`Tag palette showing how constrained tags behave when already used.`,...w.parameters?.docs?.description}}},T.parameters={...T.parameters,docs:{...T.parameters?.docs,source:{originalSource:`{
  render: () => {
    const validResult: TagValidationResult = {
      valid: true,
      errors: [],
      warnings: []
    };
    const missingRequired: TagValidationResult = {
      valid: false,
      errors: [{
        type: "deleted_non_deletable",
        message: 'Missing 1 non-deletable placeholder "Variable" tag'
      }, {
        type: "deleted_non_deletable",
        message: 'Missing 1 non-deletable placeholder "Line Break" tag'
      }],
      warnings: []
    };
    const duplicatedNonCloneable: TagValidationResult = {
      valid: false,
      errors: [{
        type: "cloned_non_cloneable",
        message: 'Duplicated 1 non-cloneable placeholder "Variable" tag'
      }],
      warnings: []
    };
    const mixedIssues: TagValidationResult = {
      valid: false,
      errors: [{
        type: "deleted_non_deletable",
        message: 'Missing 1 non-deletable placeholder "Placeholder" tag'
      }, {
        type: "unpaired",
        message: 'Closing "Bold" without matching opening tag'
      }],
      warnings: [{
        type: "extra_tag",
        message: 'Extra 1 opening "Italic" tag'
      }]
    };
    return <div style={{
      maxWidth: 640,
      padding: 16
    }}>
        <h3 style={titleStyle}>Constraint Violation Messages</h3>
        <p style={descStyle}>
          The validation bar shows real-time feedback when constraints are violated. Non-deletable
          tags generate errors when missing. Non-cloneable tags generate errors when duplicated.
        </p>

        <div style={sectionStyle}>
          <div style={sectionLabel}>All tags present (valid):</div>
          <TagValidationBar validation={validResult} />
          <p style={hintStyle}>No validation bar shown — clean state.</p>
        </div>

        <div style={sectionStyle}>
          <div style={sectionLabel}>Required tags missing:</div>
          <TagValidationBar validation={missingRequired} />
        </div>

        <div style={sectionStyle}>
          <div style={sectionLabel}>Non-cloneable tag duplicated:</div>
          <TagValidationBar validation={duplicatedNonCloneable} />
        </div>

        <div style={sectionStyle}>
          <div style={sectionLabel}>Multiple issues:</div>
          <TagValidationBar validation={mixedIssues} />
        </div>
      </div>;
  }
}`,...T.parameters?.docs?.source},description:{story:`Validation bar showing constraint violations.`,...T.parameters?.docs?.description}}},E.parameters={...E.parameters,docs:{...E.parameters?.docs,source:{originalSource:`{
  render: () => {
    const sourceText = \`Hello \${P}, you have \${P} new \${O}messages\${C} in your \${O}inbox\${C}.\${P}Check them now.\`;
    const sourceSpans: SpanInfo[] = [{
      span_type: "placeholder",
      type: "code:variable",
      id: "1",
      data: "{userName}",
      display_text: "{userName}",
      deletable: false,
      cloneable: false,
      can_reorder: true
    }, {
      span_type: "placeholder",
      type: "code:placeholder",
      id: "2",
      data: "{count}",
      display_text: "{count}",
      deletable: false,
      cloneable: false,
      can_reorder: true
    }, {
      span_type: "opening",
      type: "fmt:bold",
      id: "3",
      data: "<b>"
    }, {
      span_type: "closing",
      type: "fmt:bold",
      id: "3",
      data: "</b>"
    }, {
      span_type: "opening",
      type: "link:hyperlink",
      id: "4",
      data: '<a href="/inbox">'
    }, {
      span_type: "closing",
      type: "link:hyperlink",
      id: "4",
      data: "</a>"
    }, {
      span_type: "placeholder",
      type: "struct:break",
      id: "5",
      data: "<br/>",
      deletable: false,
      cloneable: false,
      can_reorder: false
    }];
    return <div style={{
      maxWidth: 700,
      padding: 16
    }}>
        <h3 style={titleStyle}>Real-World: Notification Message</h3>
        <p style={descStyle}>
          A typical notification string mixing variables, formatting, links, and a line break. The
          legend shows how each type is constrained.
        </p>

        <div style={{
        marginBottom: 16
      }}>
          <div style={sectionLabel}>Formatted view</div>
          <div style={{
          fontSize: 14,
          lineHeight: 1.8
        }}>
            <FormattedSourceDisplay codedText={sourceText} spans={sourceSpans} />
          </div>
        </div>

        <div style={{
        marginBottom: 16
      }}>
          <div style={sectionLabel}>Code view</div>
          <div style={{
          fontSize: 14,
          lineHeight: 1.8
        }}>
            <SourceCellDisplay codedText={sourceText} spans={sourceSpans} />
          </div>
        </div>

        <div style={{
        marginBottom: 16
      }}>
          <div style={sectionLabel}>Tag palette</div>
          <TagPalette sourceSpans={sourceSpans} onInsert={() => {}} showCategoryGroups />
        </div>

        <div>
          <InlineCodeLegend spans={sourceSpans} onClose={() => {}} />
        </div>
      </div>;
  }
}`,...E.parameters?.docs?.source},description:{story:`Full editing scenario showing how constraints protect content.`,...E.parameters?.docs?.description}}},F=[`ConstraintOverview`,`PaletteConstraintBehavior`,`ConstraintViolations`,`RealWorldScenario`]}))();export{C as ConstraintOverview,T as ConstraintViolations,w as PaletteConstraintBehavior,E as RealWorldScenario,F as __namedExportsOrder,h as default};