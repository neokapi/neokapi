"use client";

import styles from "./page.module.css";

const userName = "Sam";

const notes = [
  { id: 1, title: "Q3 launch plan", excerpt: "Rollout timeline, owners, and the comms checklist." },
  { id: 2, title: "Design review", excerpt: "Tighten the empty states and ship the new icon set." },
  { id: 3, title: "Reading list", excerpt: "Three papers on incremental computation to get through." },
];

export default function Home() {
  return (
    <main className={styles.wrap}>
      <header className={styles.header}>
        <h1 className={styles.title}>Your notes</h1>
        <p className={styles.subtitle}>Welcome back, {userName}.</p>
      </header>

      <section className={styles.actions}>
        <button className={styles.newBtn} onClick={() => alert("New note")}>
          New note
        </button>
        <div className={styles.search}>Search your notes</div>
      </section>

      <ul className={styles.list}>
        {notes.map((n) => (
          <li key={n.id} className={styles.card}>
            <div className={styles.cardTitle}>{n.title}</div>
            <div className={styles.cardExcerpt}>{n.excerpt}</div>
          </li>
        ))}
      </ul>

      <footer className={styles.footer}>Lumen Notes keeps your ideas in sync across every device.</footer>
    </main>
  );
}
