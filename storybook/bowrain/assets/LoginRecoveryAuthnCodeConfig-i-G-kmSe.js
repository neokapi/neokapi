import{a as e,n as t}from"./chunk-DnJy8xQt.js";import{t as n}from"./react-Baqbuk-D.js";import{t as r}from"./jsx-runtime-B-hFyic3.js";import{i,n as a,r as o,t as s}from"./kcClsx-DZ0ocyQA.js";import{n as c,t as l}from"./useInsertScriptTags-DyG7n_Te.js";import{n as u,t as d}from"./waitForElementMountedOnDom-C7bQRbIB.js";function f(e){let{olRecoveryCodesListId:t,i18n:n}=e,{msgStr:r,isFetchingTranslations:i}=n,{insertScriptTags:a}=c({componentOrHookName:`LoginRecoveryAuthnCodeConfig`,scriptTags:[{type:`text/javascript`,textContent:()=>`

                    /* copy recovery codes  */
                    function copyRecoveryCodes() {
                        var tmpTextarea = document.createElement("textarea");
                        var codes = document.querySelectorAll("#${t} li");
                        for (i = 0; i < codes.length; i++) {
                            tmpTextarea.value = tmpTextarea.value + codes[i].innerText + "\\n";
                        }
                        document.body.appendChild(tmpTextarea);
                        tmpTextarea.select();
                        document.execCommand("copy");
                        document.body.removeChild(tmpTextarea);
                    }

                    var copyButton = document.getElementById("copyRecoveryCodes");
                    copyButton && copyButton.addEventListener("click", function () {
                        copyRecoveryCodes();
                    });

                    /* download recovery codes  */
                    function formatCurrentDateTime() {
                        var dt = new Date();
                        var options = {
                            month: 'long',
                            day: 'numeric',
                            year: 'numeric',
                            hour: 'numeric',
                            minute: 'numeric',
                            timeZoneName: 'short'
                        };

                        return dt.toLocaleString('en-US', options);
                    }

                    function parseRecoveryCodeList() {
                        var recoveryCodes = document.querySelectorAll("#${t} li");
                        var recoveryCodeList = "";

                        for (var i = 0; i < recoveryCodes.length; i++) {
                            var recoveryCodeLiElement = recoveryCodes[i].innerText;
                            recoveryCodeList += recoveryCodeLiElement + "\\r\\n";
                        }

                        return recoveryCodeList;
                    }

                    function buildDownloadContent() {
                        var recoveryCodeList = parseRecoveryCodeList();
                        var dt = new Date();
                        var options = {
                            month: 'long',
                            day: 'numeric',
                            year: 'numeric',
                            hour: 'numeric',
                            minute: 'numeric',
                            timeZoneName: 'short'
                        };

                        return fileBodyContent =
                            ${JSON.stringify(r(`recovery-codes-download-file-header`))} + "\\n\\n" +
                            recoveryCodeList + "\\n" +
                            ${JSON.stringify(r(`recovery-codes-download-file-description`))} + "\\n\\n" +
                            ${JSON.stringify(r(`recovery-codes-download-file-date`))} + " " + formatCurrentDateTime();
                    }

                    function setUpDownloadLinkAndDownload(filename, text) {
                        var el = document.createElement('a');
                        el.setAttribute('href', 'data:text/plain;charset=utf-8,' + encodeURIComponent(text));
                        el.setAttribute('download', filename);
                        el.style.display = 'none';
                        document.body.appendChild(el);
                        el.click();
                        document.body.removeChild(el);
                    }

                    function downloadRecoveryCodes() {
                        setUpDownloadLinkAndDownload('kc-download-recovery-codes.txt', buildDownloadContent());
                    }

                    var downloadButton = document.getElementById("downloadRecoveryCodes");
                    downloadButton && downloadButton.addEventListener("click", downloadRecoveryCodes);

                    /* print recovery codes */
                    function buildPrintContent() {
                        var recoveryCodeListHTML = document.getElementById('${t}').innerHTML;
                        var styles =
                            \`@page { size: auto;  margin-top: 0; }
                            body { width: 480px; }
                            div { list-style-type: none; font-family: monospace }
                            p:first-of-type { margin-top: 48px }\`;

                        return printFileContent =
                            "<html><style>" + styles + "</style><body>" +
                            "<title>kc-download-recovery-codes</title>" +
                            "<p>" + ${JSON.stringify(r(`recovery-codes-download-file-header`))} + "</p>" +
                            "<div>" + recoveryCodeListHTML + "</div>" +
                            "<p>" + ${JSON.stringify(r(`recovery-codes-download-file-description`))} + "</p>" +
                            "<p>" + ${JSON.stringify(r(`recovery-codes-download-file-date`))} + " " + formatCurrentDateTime() + "</p>" +
                            "</body></html>";
                    }

                    function printRecoveryCodes() {
                        var w = window.open();
                        w.document.write(buildPrintContent());
                        w.print();
                        w.close();
                    }

                    var printButton = document.getElementById("printRecoveryCodes");
                    printButton && printButton.addEventListener("click", printRecoveryCodes);
                `}]});(0,p.useEffect)(()=>{i||(async()=>{await u({elementId:t}),a()})()},[i])}var p,m=t((()=>{p=e(n()),l(),d()}));function h(e){let{kcContext:t,i18n:n,doUseDefaultCss:r,Template:i,classes:a}=e,{kcClsx:c}=s({doUseDefaultCss:r,classes:a}),{recoveryAuthnCodesConfigBean:l,isAppInitiatedAction:u}=t,{msg:d,msgStr:p}=n,m=`kc-recovery-codes-list`;return f({olRecoveryCodesListId:m,i18n:n}),(0,_.jsxs)(i,Object.assign({kcContext:t,i18n:n,doUseDefaultCss:r,classes:a,headerNode:d(`recovery-code-config-header`)},{children:[(0,_.jsxs)(`div`,Object.assign({className:o(`pf-c-alert`,`pf-m-warning`,`pf-m-inline`,c(`kcRecoveryCodesWarning`)),"aria-label":`Warning alert`},{children:[(0,_.jsx)(`div`,Object.assign({className:`pf-c-alert__icon`},{children:(0,_.jsx)(`i`,{className:`pficon-warning-triangle-o`,"aria-hidden":`true`})})),(0,_.jsxs)(`h4`,Object.assign({className:`pf-c-alert__title`},{children:[(0,_.jsx)(`span`,Object.assign({className:`pf-screen-reader`},{children:`Warning alert:`})),d(`recovery-code-config-warning-title`)]})),(0,_.jsx)(`div`,Object.assign({className:`pf-c-alert__description`},{children:(0,_.jsx)(`p`,{children:d(`recovery-code-config-warning-message`)})}))]})),(0,_.jsx)(`ol`,Object.assign({id:m,className:c(`kcRecoveryCodesList`)},{children:l.generatedRecoveryAuthnCodesList.map((e,t)=>(0,_.jsxs)(`li`,{children:[(0,_.jsxs)(`span`,{children:[t+1,`:`]}),` `,e.slice(0,4),`-`,e.slice(4,8),`-`,e.slice(8)]},t))})),(0,_.jsxs)(`div`,Object.assign({className:c(`kcRecoveryCodesActions`)},{children:[(0,_.jsxs)(`button`,Object.assign({id:`printRecoveryCodes`,className:o(`pf-c-button`,`pf-m-link`),type:`button`},{children:[(0,_.jsx)(`i`,{className:`pficon-print`,"aria-hidden":`true`}),` `,d(`recovery-codes-print`)]})),(0,_.jsxs)(`button`,Object.assign({id:`downloadRecoveryCodes`,className:o(`pf-c-button`,`pf-m-link`),type:`button`},{children:[(0,_.jsx)(`i`,{className:`pficon-save`,"aria-hidden":`true`}),` `,d(`recovery-codes-download`)]})),(0,_.jsxs)(`button`,Object.assign({id:`copyRecoveryCodes`,className:o(`pf-c-button`,`pf-m-link`),type:`button`},{children:[(0,_.jsx)(`i`,{className:`pficon-blueprint`,"aria-hidden":`true`}),` `,d(`recovery-codes-copy`)]}))]})),(0,_.jsxs)(`div`,Object.assign({className:c(`kcFormOptionsClass`)},{children:[(0,_.jsx)(`input`,{className:c(`kcCheckInputClass`),type:`checkbox`,id:`kcRecoveryCodesConfirmationCheck`,name:`kcRecoveryCodesConfirmationCheck`,onChange:e=>{document.getElementById(`saveRecoveryAuthnCodesBtn`).disabled=!e.target.checked}}),(0,_.jsx)(`label`,Object.assign({htmlFor:`kcRecoveryCodesConfirmationCheck`},{children:d(`recovery-codes-confirmation-message`)}))]})),(0,_.jsxs)(`form`,Object.assign({action:t.url.loginAction,className:c(`kcFormGroupClass`),id:`kc-recovery-codes-settings-form`,method:`post`},{children:[(0,_.jsx)(`input`,{type:`hidden`,name:`generatedRecoveryAuthnCodes`,value:l.generatedRecoveryAuthnCodesAsString}),(0,_.jsx)(`input`,{type:`hidden`,name:`generatedAt`,value:l.generatedAt}),(0,_.jsx)(`input`,{type:`hidden`,id:`userLabel`,name:`userLabel`,value:p(`recovery-codes-label-default`)}),(0,_.jsx)(g,{kcClsx:c,i18n:n}),u?(0,_.jsxs)(_.Fragment,{children:[(0,_.jsx)(`input`,{type:`submit`,className:c(`kcButtonClass`,`kcButtonPrimaryClass`,`kcButtonLargeClass`),id:`saveRecoveryAuthnCodesBtn`,value:p(`recovery-codes-action-complete`),disabled:!0}),(0,_.jsx)(`button`,Object.assign({type:`submit`,className:c(`kcButtonClass`,`kcButtonDefaultClass`,`kcButtonLargeClass`),id:`cancelRecoveryAuthnCodesBtn`,name:`cancel-aia`,value:`true`},{children:d(`recovery-codes-action-cancel`)}))]}):(0,_.jsx)(`input`,{type:`submit`,className:c(`kcButtonClass`,`kcButtonPrimaryClass`,`kcButtonBlockClass`,`kcButtonLargeClass`),id:`saveRecoveryAuthnCodesBtn`,value:p(`recovery-codes-action-complete`),disabled:!0})]}))]}))}function g(e){let{kcClsx:t,i18n:n}=e,{msg:r}=n;return(0,_.jsx)(`div`,Object.assign({id:`kc-form-options`,className:t(`kcFormOptionsClass`)},{children:(0,_.jsx)(`div`,Object.assign({className:t(`kcFormOptionsWrapperClass`)},{children:(0,_.jsx)(`div`,Object.assign({className:`checkbox`},{children:(0,_.jsxs)(`label`,{children:[(0,_.jsx)(`input`,{type:`checkbox`,id:`logout-sessions`,name:`logout-sessions`,value:`on`,defaultChecked:!0}),r(`logoutOtherSessions`)]})}))}))}))}var _;t((()=>{_=r(),i(),a(),m()}))();export{h as default};