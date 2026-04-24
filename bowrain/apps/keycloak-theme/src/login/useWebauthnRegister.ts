/**
 * Custom WebAuthn registration script hook that skips the "name your key"
 * window.prompt(). Instead, auto-generates a label from the browser/platform
 * (e.g., "Passkey — Chrome on macOS").
 *
 * Based on keycloakify's useScript but patches the registerByWebAuthn flow
 * to set the authenticatorLabel directly without user interaction.
 */
import { useEffect } from "react";
import { useInsertScriptTags } from "keycloakify/tools/useInsertScriptTags";
import { waitForElementMountedOnDom } from "keycloakify/tools/waitForElementMountedOnDom";

type KcContextLike = {
  url: { resourcesPath: string };
  challenge: string;
  userid: string;
  username: string;
  signatureAlgorithms: string[];
  rpEntityName: string;
  rpId: string;
  attestationConveyancePreference: string;
  authenticatorAttachment: string;
  requireResidentKey: string;
  userVerificationRequirement: string;
  createTimeout: number | string;
  excludeCredentialIds: string;
};

type I18nLike = {
  msgStr: (
    key:
      | "webauthn-registration-init-label"
      | "webauthn-registration-init-label-prompt"
      | "webauthn-unsupported-browser-text",
  ) => string;
  isFetchingTranslations: boolean;
};

/**
 * Generates a human-friendly passkey label from the user agent.
 * Examples: "Chrome on macOS", "Safari on iPhone", "Firefox on Windows"
 */
function generatePasskeyLabel(): string {
  const ua = navigator.userAgent;
  let browser = "Browser";
  let platform = "Device";

  if (ua.includes("Chrome") && !ua.includes("Edg")) browser = "Chrome";
  else if (ua.includes("Edg")) browser = "Edge";
  else if (ua.includes("Safari") && !ua.includes("Chrome")) browser = "Safari";
  else if (ua.includes("Firefox")) browser = "Firefox";

  if (ua.includes("Mac OS")) platform = "macOS";
  else if (ua.includes("Windows")) platform = "Windows";
  else if (ua.includes("Linux") && !ua.includes("Android")) platform = "Linux";
  else if (ua.includes("Android")) platform = "Android";
  else if (ua.includes("iPhone") || ua.includes("iPad")) platform = "iOS";

  return `${browser} on ${platform}`;
}

export function useWebauthnRegisterScript(params: {
  authButtonId: string;
  kcContext: KcContextLike;
  i18n: I18nLike;
}) {
  const { authButtonId, kcContext, i18n } = params;

  const {
    url,
    challenge,
    userid,
    username,
    signatureAlgorithms,
    rpEntityName,
    rpId,
    attestationConveyancePreference,
    authenticatorAttachment,
    requireResidentKey,
    userVerificationRequirement,
    createTimeout,
    excludeCredentialIds,
  } = kcContext;

  const { isFetchingTranslations } = i18n;

  // We inject a patched version of the registration flow that:
  // 1. Calls registerByWebAuthn (standard credential creation)
  // 2. Intercepts window.prompt to return auto-generated label instead
  //
  // Keycloak's webauthnRegister.js always calls window.prompt(initLabelPrompt,
  // initLabel) after credential creation, even when we pass an empty
  // initLabelPrompt. To avoid that extra "name your passkey" dialog, we
  // replace window.prompt wholesale while the registration flow is running
  // and return our auto-generated label. The original prompt is restored
  // after the request submits (or on failure) so nothing else is affected.
  const autoLabel = generatePasskeyLabel();
  const { insertScriptTags } = useInsertScriptTags({
    componentOrHookName: "WebauthnRegisterAutoLabel",
    scriptTags: [
      {
        type: "module",
        textContent: () => `
          import { registerByWebAuthn } from "${url.resourcesPath}/js/webauthnRegister.js";

          const AUTO_LABEL = ${JSON.stringify(autoLabel)};

          const registerButton = document.getElementById('${authButtonId}');
          registerButton.addEventListener("click", function() {
            const originalPrompt = window.prompt;
            window.prompt = function() { return AUTO_LABEL; };

            const restorePrompt = function() { window.prompt = originalPrompt; };
            // registerByWebAuthn submits the form on success and never resolves;
            // on failure the button remains clickable, so restore after a tick.
            setTimeout(restorePrompt, 30000);

            const input = {
              challenge : '${challenge}',
              userid : '${userid}',
              username : '${username}',
              signatureAlgorithms : ${JSON.stringify(signatureAlgorithms)},
              rpEntityName : ${JSON.stringify(rpEntityName)},
              rpId : ${JSON.stringify(rpId)},
              attestationConveyancePreference : ${JSON.stringify(attestationConveyancePreference)},
              authenticatorAttachment : ${JSON.stringify(authenticatorAttachment)},
              requireResidentKey : ${JSON.stringify(requireResidentKey)},
              userVerificationRequirement : ${JSON.stringify(userVerificationRequirement)},
              createTimeout : ${createTimeout},
              excludeCredentialIds : ${JSON.stringify(excludeCredentialIds)},
              initLabel : AUTO_LABEL,
              initLabelPrompt : '',
              errmsg : 'Your browser does not support passkeys.'
            };
            registerByWebAuthn(input);
          });
        `,
      },
    ],
  });

  useEffect(() => {
    if (isFetchingTranslations) {
      return;
    }

    void (async () => {
      await waitForElementMountedOnDom({ elementId: authButtonId });
      insertScriptTags();
    })();
  }, [isFetchingTranslations]);
}
