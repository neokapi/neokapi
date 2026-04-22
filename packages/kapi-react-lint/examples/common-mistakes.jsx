/**
 * Fixture of the common kapi-react authoring mistakes each rule catches.
 * Used by tests (as integration input) and by docs (as copy-paste).
 *
 * Every offending line has an inline comment pointing at the rule.
 * Kept as .jsx (no TS type syntax) so it parses with the default
 * ESLint parser and doesn't require a toolchain in the minimal example.
 */
import { t } from "@neokapi/kapi-react/runtime";

export function Mistakes({ name, brand, fileName }) {
  // t-literal-first-arg
  const a = t(name);

  // t-no-concat
  const b = t("Hello " + name);
  const c = t(`Hello ${name}`);

  // no-concat-in-translatable-attr
  const d = <img alt={"Logo " + brand} />;
  const e = <button title={`Save ${fileName}`}>Save</button>;

  // no-string-literal-jsx-expr
  const f = <p>{"Hello"}</p>;

  // prefer-t-for-label-props (only with recommended-strict)
  const THEMES = [
    { value: "system", label: "System" },
    { value: "light", label: "Light" },
  ];

  return (
    <>
      {a}
      {b}
      {c}
      {d}
      {e}
      {f}
      {THEMES.map(({ value, label }) => (
        <button key={value}>{label}</button>
      ))}
    </>
  );
}
