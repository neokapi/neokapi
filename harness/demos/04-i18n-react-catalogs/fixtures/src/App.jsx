import { useTranslation } from "react-i18next";

export default function App() {
  const { t } = useTranslation();
  const count = 3;
  return (
    <main>
      <nav>
        <a href="/">{t("nav.dashboard")}</a>
        <a href="/tasks">{t("nav.tasks")}</a>
      </nav>
      <h1>{t("greeting", { name: "Alex" })}</h1>
      <p>{t("tasks.count", { count })}</p>
      <button>{t("tasks.add")}</button>
      <label>
        <input type="checkbox" /> {t("settings.notifications")}
      </label>
    </main>
  );
}
