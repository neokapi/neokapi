import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{t as i}from"./label-HrqY8KiI.js";import{t as a}from"./button-MBstDm8k.js";import{a as o,o as s,r as c,t as l}from"./card-p3PnhtGK.js";import{t as u}from"./iframe-Cr-Gz2Kj.js";import{n as d,t as f}from"./useInsertScriptTags-DyG7n_Te.js";import{n as p,t as m}from"./waitForElementMountedOnDom-C7bQRbIB.js";function h(){let e=navigator.userAgent,t=`Browser`,n=`Device`;return e.includes(`Chrome`)&&!e.includes(`Edg`)?t=`Chrome`:e.includes(`Edg`)?t=`Edge`:e.includes(`Safari`)&&!e.includes(`Chrome`)?t=`Safari`:e.includes(`Firefox`)&&(t=`Firefox`),e.includes(`Mac OS`)?n=`macOS`:e.includes(`Windows`)?n=`Windows`:e.includes(`Linux`)&&!e.includes(`Android`)?n=`Linux`:e.includes(`Android`)?n=`Android`:(e.includes(`iPhone`)||e.includes(`iPad`))&&(n=`iOS`),`${t} on ${n}`}function g(e){let{authButtonId:t,kcContext:n,i18n:r}=e,{url:i,challenge:a,userid:o,username:s,signatureAlgorithms:c,rpEntityName:l,rpId:u,attestationConveyancePreference:f,authenticatorAttachment:m,requireResidentKey:g,userVerificationRequirement:v,createTimeout:y,excludeCredentialIds:b}=n,{isFetchingTranslations:x}=r,S=h(),{insertScriptTags:C}=d({componentOrHookName:`WebauthnRegisterAutoLabel`,scriptTags:[{type:`module`,textContent:()=>`
          import { registerByWebAuthn } from "${i.resourcesPath}/js/webauthnRegister.js";

          const AUTO_LABEL = ${JSON.stringify(S)};

          const registerButton = document.getElementById('${t}');
          registerButton.addEventListener("click", function() {
            const originalPrompt = window.prompt;
            window.prompt = function() { return AUTO_LABEL; };

            const restorePrompt = function() { window.prompt = originalPrompt; };
            // registerByWebAuthn submits the form on success and never resolves;
            // on failure the button remains clickable, so restore after a tick.
            setTimeout(restorePrompt, 30000);

            const input = {
              challenge : '${a}',
              userid : '${o}',
              username : '${s}',
              signatureAlgorithms : ${JSON.stringify(c)},
              rpEntityName : ${JSON.stringify(l)},
              rpId : ${JSON.stringify(u)},
              attestationConveyancePreference : ${JSON.stringify(f)},
              authenticatorAttachment : ${JSON.stringify(m)},
              requireResidentKey : ${JSON.stringify(g)},
              userVerificationRequirement : ${JSON.stringify(v)},
              createTimeout : ${y},
              excludeCredentialIds : ${JSON.stringify(b)},
              initLabel : AUTO_LABEL,
              initLabelPrompt : '',
              errmsg : 'Your browser does not support passkeys.'
            };
            registerByWebAuthn(input);
          });
        `}]});(0,_.useEffect)(()=>{x||(async()=>{await p({elementId:t}),C()})()},[x])}var _,v=t((()=>{_=e(n(),1),f(),m()}));function y(e){let{kcContext:t,i18n:n}=e,{url:r,isSetRetry:u,isAppInitiatedAction:d}=t,{msg:f,msgStr:p}=n,m=`authenticateWebAuthnButton`;return g({authButtonId:m,kcContext:t,i18n:n}),(0,x.jsxs)(`div`,{className:`w-full max-w-md px-4`,children:[(0,x.jsx)(`div`,{className:`flex justify-center mb-8`,children:(0,x.jsx)(b,{})}),(0,x.jsxs)(l,{children:[(0,x.jsxs)(s,{className:`text-center space-y-1 pb-2`,children:[(0,x.jsx)(`h1`,{className:`text-2xl font-semibold tracking-tight`,children:f(`webauthn-registration-title`)}),(0,x.jsx)(`p`,{className:`text-sm text-muted-foreground`,children:`Touch your security key or use your device biometrics to register a passkey.`})]}),(0,x.jsxs)(c,{className:`space-y-4`,children:[(0,x.jsxs)(`form`,{id:`register`,action:r.loginAction,method:`post`,children:[(0,x.jsx)(`input`,{type:`hidden`,id:`clientDataJSON`,name:`clientDataJSON`}),(0,x.jsx)(`input`,{type:`hidden`,id:`attestationObject`,name:`attestationObject`}),(0,x.jsx)(`input`,{type:`hidden`,id:`publicKeyCredentialId`,name:`publicKeyCredentialId`}),(0,x.jsx)(`input`,{type:`hidden`,id:`authenticatorLabel`,name:`authenticatorLabel`}),(0,x.jsx)(`input`,{type:`hidden`,id:`transports`,name:`transports`}),(0,x.jsx)(`input`,{type:`hidden`,id:`error`,name:`error`}),(0,x.jsxs)(`div`,{className:`flex items-center gap-2`,children:[(0,x.jsx)(`input`,{type:`checkbox`,id:`logout-sessions`,name:`logout-sessions`,value:`on`,defaultChecked:!0,className:`h-4 w-4 rounded border-border`}),(0,x.jsx)(i,{htmlFor:`logout-sessions`,className:`text-sm font-normal`,children:f(`logoutOtherSessions`)})]})]}),(0,x.jsx)(a,{id:m,type:`button`,className:`w-full`,children:p(`doRegisterSecurityKey`)}),!u&&d&&(0,x.jsx)(`form`,{action:r.loginAction,method:`post`,children:(0,x.jsx)(a,{type:`submit`,variant:`secondary`,className:`w-full`,name:`cancel-aia`,value:`true`,children:p(`doCancel`)})})]}),(0,x.jsx)(o,{})]})]})}function b(){return(0,x.jsx)(`img`,{src:S,width:`48`,height:`48`,alt:`Bowrain`,className:`rounded-xl`})}var x,S;t((()=>{x=r(),u(),v(),S=`/bowrain/storybook/logo.png`,y.__docgenInfo={description:``,methods:[],displayName:`WebauthnRegister`,props:{kcContext:{required:!0,tsType:{name:`Extract`,elements:[{name:`ExtendKcContext`,elements:[{name:`signature`,type:`object`,raw:`{
  themeName: ThemeName;
  properties: Record<KcEnvName, string>;
}`,signature:{properties:[{key:`themeName`,value:{name:`literal`,value:`"bowrain"`,required:!0}},{key:`properties`,value:{name:`Record`,elements:[{name:`never`},{name:`string`}],raw:`Record<KcEnvName, string>`,required:!0}}]}},{name:`signature`,type:`object`,raw:`{}`,signature:{properties:[]}}],raw:`ExtendKcContext<KcContextExtension, KcContextExtensionPerPage>`},{name:`signature`,type:`object`,raw:`{ pageId: "webauthn-register.ftl" }`,signature:{properties:[{key:`pageId`,value:{name:`literal`,value:`"webauthn-register.ftl"`,required:!0}}]}}],raw:`Extract<KcContext, { pageId: "webauthn-register.ftl" }>`},description:``},i18n:{required:!0,tsType:{name:`ofTypeI18n`},description:``}}}}))();export{y as default};