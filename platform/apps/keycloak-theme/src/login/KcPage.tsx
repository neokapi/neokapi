import { Suspense, lazy } from "react";
import type { ClassKey } from "keycloakify/login";
import type { KcContext } from "./KcContext";
import { useI18n } from "./i18n";
import DefaultPage from "keycloakify/login/DefaultPage";
import Template from "keycloakify/login/Template";
import { AnimatedBackgroundGlass } from "@gokapi/ui/components/ui/animated-background";
import "./main.css";

const Login = lazy(() => import("./pages/Login"));
const Register = lazy(() => import("./pages/Register"));
const ErrorPage = lazy(() => import("./pages/Error"));
const UserProfileFormFields = lazy(
    () => import("keycloakify/login/UserProfileFormFields")
);

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
                            return <Login kcContext={kcContext} i18n={i18n} />;
                        case "register.ftl":
                            return <Register kcContext={kcContext} i18n={i18n} />;
                        case "error.ftl":
                            return <ErrorPage kcContext={kcContext} i18n={i18n} />;
                        default:
                            return (
                                <DefaultPage
                                    kcContext={kcContext}
                                    i18n={i18n}
                                    classes={classes}
                                    Template={Template}
                                    doUseDefaultCss={false}
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
