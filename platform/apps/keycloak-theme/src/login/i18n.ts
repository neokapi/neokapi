import { i18nBuilder } from "keycloakify/login";
import type { ThemeName } from "../kc.gen";

const { useI18n, ofTypeI18n } = i18nBuilder
  .withThemeName<ThemeName>()
  .withCustomTranslations({
    en: {
      backToLogin: "Back to Login",
      backToApplication: "Back to Application",
    },
    fr: {
      backToLogin: "Retour à la connexion",
      backToApplication: "Retour à l''application",
    },
    de: {
      backToLogin: "Zurück zur Anmeldung",
      backToApplication: "Zurück zur Anwendung",
    },
    es: {
      backToLogin: "Volver al inicio de sesión",
      backToApplication: "Volver a la aplicación",
    },
  })
  .build();

type I18n = typeof ofTypeI18n;

export { useI18n, type I18n };
