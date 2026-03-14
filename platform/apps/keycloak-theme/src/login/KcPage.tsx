import { Suspense, lazy } from "react";
import type { ClassKey } from "keycloakify/login";
import type { KcContext } from "./KcContext";
import { useI18n } from "./i18n";
import DefaultPage from "keycloakify/login/DefaultPage";
import Template from "keycloakify/login/Template";
import { AnimatedBackgroundGlass } from "@neokapi/ui/components/ui/animated-background";
import "./main.css";

const Login = lazy(() => import("./pages/Login"));
const Register = lazy(() => import("./pages/Register"));
const ErrorPage = lazy(() => import("./pages/Error"));
const Info = lazy(() => import("./pages/Info"));
const LoginVerifyEmail = lazy(() => import("./pages/LoginVerifyEmail"));
const LogoutConfirm = lazy(() => import("./pages/LogoutConfirm"));
const LoginPageExpired = lazy(() => import("./pages/LoginPageExpired"));
const LoginIdpLinkConfirm = lazy(() => import("./pages/LoginIdpLinkConfirm"));
const LoginIdpLinkEmail = lazy(() => import("./pages/LoginIdpLinkEmail"));
const LoginPasskeysConditionalAuthenticate = lazy(() => import("./pages/LoginPasskeysConditionalAuthenticate"));
const WebauthnAuthenticate = lazy(() => import("./pages/WebauthnAuthenticate"));
const WebauthnRegister = lazy(() => import("./pages/WebauthnRegister"));
const WebauthnError = lazy(() => import("./pages/WebauthnError"));
const UserProfileFormFields = lazy(() => import("keycloakify/login/UserProfileFormFields"));

export default function KcPage(props: { kcContext: KcContext }) {
  const { kcContext } = props;
  const { i18n } = useI18n({ kcContext });

  return (
    <>
      <AnimatedBackgroundGlass />
      <Suspense>
        {(() => {
          switch (kcContext.pageId) {
            case "login.ftl":
            case "login-username.ftl":
              return <Login kcContext={kcContext} i18n={i18n} />;
            case "register.ftl":
              return <Register kcContext={kcContext} i18n={i18n} />;
            case "error.ftl":
              return <ErrorPage kcContext={kcContext} i18n={i18n} />;
            case "info.ftl":
              return <Info kcContext={kcContext} i18n={i18n} />;
            case "login-verify-email.ftl":
              return <LoginVerifyEmail kcContext={kcContext} i18n={i18n} />;
            case "logout-confirm.ftl":
              return <LogoutConfirm kcContext={kcContext} i18n={i18n} />;
            case "login-page-expired.ftl":
              return <LoginPageExpired kcContext={kcContext} i18n={i18n} />;
            case "login-idp-link-confirm.ftl":
              return <LoginIdpLinkConfirm kcContext={kcContext} i18n={i18n} />;
            case "login-idp-link-email.ftl":
              return <LoginIdpLinkEmail kcContext={kcContext} i18n={i18n} />;
            case "login-passkeys-conditional-authenticate.ftl":
              return <LoginPasskeysConditionalAuthenticate kcContext={kcContext} i18n={i18n} />;
            case "webauthn-authenticate.ftl":
              return <WebauthnAuthenticate kcContext={kcContext} i18n={i18n} />;
            case "webauthn-register.ftl":
              return <WebauthnRegister kcContext={kcContext} i18n={i18n} />;
            case "webauthn-error.ftl":
              return <WebauthnError kcContext={kcContext} i18n={i18n} />;
            default:
              return (
                <DefaultPage
                  kcContext={kcContext}
                  i18n={i18n}
                  classes={classes}
                  Template={Template}
                  doUseDefaultCss={true}
                  UserProfileFormFields={UserProfileFormFields}
                  doMakeUserConfirmPassword={true}
                />
              );
          }
        })()}
      </Suspense>
    </>
  );
}

const classes = {} satisfies { [key in ClassKey]?: string };
