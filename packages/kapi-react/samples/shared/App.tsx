/**
 * Sample React component — vanilla JSX, no i18n imports.
 *
 * The @neokapi/kapi-react plugin handles translation automatically.
 * This file is identical across all sample projects.
 */

export default function App({ user = { name: 'Alice' }, count = 3 }) {
  return (
    <div>
      <h1>Welcome back, {user.name}!</h1>
      <p>You have {count} unread messages.</p>
      <p>
        Click <a href="/settings">here</a> to manage your account.
      </p>
      <input placeholder="Search messages..." />
      <button>Save changes</button>

      {/* Not translated — translate="no" */}
      <code translate="no">API_KEY_PREFIX</code>
    </div>
  );
}
