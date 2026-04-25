import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{n as i,t as a}from"./Pagination-Cnw9_SWA.js";var o,s,c,l,u,d,f,p;t((()=>{o=e(n(),1),i(),s=r(),c={title:`Resource Browser/Pagination`,component:a,tags:[`autodocs`],decorators:[e=>(0,s.jsx)(`div`,{style:{maxWidth:500,padding:16},children:(0,s.jsx)(e,{})})],parameters:{docs:{description:{component:`Pagination controls with 'Showing X to Y of Z' summary and Previous/Next buttons. Hidden when total pages is 1 or fewer.`}}}},l={args:{page:0,pageSize:50,totalCount:284,onPageChange:()=>{}}},u={args:{page:3,pageSize:50,totalCount:284,onPageChange:()=>{}}},d={args:{page:5,pageSize:50,totalCount:284,onPageChange:()=>{}}},f={render:function(){let[e,t]=(0,o.useState)(0);return(0,s.jsx)(a,{page:e,pageSize:50,totalCount:284,onPageChange:t})}},l.parameters={...l.parameters,docs:{...l.parameters?.docs,source:{originalSource:`{
  args: {
    page: 0,
    pageSize: 50,
    totalCount: 284,
    onPageChange: () => {}
  }
}`,...l.parameters?.docs?.source}}},u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  args: {
    page: 3,
    pageSize: 50,
    totalCount: 284,
    onPageChange: () => {}
  }
}`,...u.parameters?.docs?.source}}},d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  args: {
    page: 5,
    pageSize: 50,
    totalCount: 284,
    onPageChange: () => {}
  }
}`,...d.parameters?.docs?.source}}},f.parameters={...f.parameters,docs:{...f.parameters?.docs,source:{originalSource:`{
  render: function InteractivePagination() {
    const [page, setPage] = useState(0);
    return <Pagination page={page} pageSize={50} totalCount={284} onPageChange={setPage} />;
  }
}`,...f.parameters?.docs?.source},description:{story:`Interactive pagination that tracks the current page.`,...f.parameters?.docs?.description}}},p=[`FirstPage`,`MiddlePage`,`LastPage`,`Interactive`]}))();export{l as FirstPage,f as Interactive,d as LastPage,u as MiddlePage,p as __namedExportsOrder,c as default};