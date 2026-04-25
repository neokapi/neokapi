import{n as e}from"./chunk-DnJy8xQt.js";import{t}from"./jsx-runtime-B-hFyic3.js";import{n,t as r}from"./ApiTokenManager-DLGkYu6A.js";import{n as i,r as a,t as o}from"./decorators-BptasVUO.js";import{D as s,k as c,s as l}from"./fixtures-nZqLq3hB.js";var u,d,f,p,m,h,g,_;e((()=>{n(),i(),l(),u=t(),d=[{id:`tok-1`,user_id:`u-1`,workspace_id:`ws-1`,name:`CI/CD Pipeline`,token_prefix:`bwt_ab12`,scopes:`["*"]`,last_used_at:new Date(Date.now()-864e5).toISOString(),expires_at:new Date(Date.now()+30*864e5).toISOString(),created_at:new Date(Date.now()-7*864e5).toISOString()},{id:`tok-2`,user_id:`u-1`,workspace_id:`ws-1`,name:`Translation Bot`,token_prefix:`bwt_cd34`,scopes:`["translate:fr,de"]`,last_used_at:null,expires_at:new Date(Date.now()+90*864e5).toISOString(),created_at:new Date(Date.now()-2*864e5).toISOString()},{id:`tok-3`,user_id:`u-1`,workspace_id:`ws-1`,name:`Review Service`,token_prefix:`bwt_ef56`,scopes:`["review"]`,last_used_at:new Date(Date.now()-36e5).toISOString(),expires_at:null,created_at:new Date(Date.now()-14*864e5).toISOString()},{id:`tok-4`,user_id:`u-1`,workspace_id:`ws-1`,name:`Read-Only Monitor`,token_prefix:`bwt_gh78`,scopes:`["read"]`,last_used_at:null,expires_at:new Date(Date.now()-864e5).toISOString(),created_at:new Date(Date.now()-60*864e5).toISOString()}],f=o(void 0,{listApiTokens:async()=>d}),p={title:`Workspace/Access/ApiTokenManager`,component:r,tags:[`autodocs`],decorators:[e=>(0,u.jsx)(`div`,{style:{maxWidth:800,padding:24},children:(0,u.jsx)(e,{})})]},m={decorators:[a],args:{workspace:s}},h={decorators:[f],args:{workspace:s}},g={decorators:[a],args:{workspace:c}},m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  decorators: [withProviders],
  args: {
    workspace: sampleWorkspace
  }
}`,...m.parameters?.docs?.source},description:{story:`Owner view with empty token list.`,...m.parameters?.docs?.description}}},h.parameters={...h.parameters,docs:{...h.parameters?.docs,source:{originalSource:`{
  decorators: [withTokens],
  args: {
    workspace: sampleWorkspace
  }
}`,...h.parameters?.docs?.source},description:{story:`Owner view with tokens showing various scopes.`,...h.parameters?.docs?.description}}},g.parameters={...g.parameters,docs:{...g.parameters?.docs,source:{originalSource:`{
  decorators: [withProviders],
  args: {
    workspace: viewerWorkspace
  }
}`,...g.parameters?.docs?.source},description:{story:`Viewer — component returns null since role is not owner/admin.`,...g.parameters?.docs?.description}}},_=[`OwnerView`,`WithTokens`,`ViewerHidden`]}))();export{m as OwnerView,g as ViewerHidden,h as WithTokens,_ as __namedExportsOrder,p as default};