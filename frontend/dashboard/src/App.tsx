import "./styles.css";

const stats = [
  ["Queue depth", "0"],
  ["Throughput/min", "0"],
  ["Workers", "0"],
  ["Dead jobs", "0"],
];

export default function App() {
  return (
    <main className="dashboard-shell">
      <header>
        <p className="eyebrow">Transcodex Dashboard</p>
        <h1>Pipeline health</h1>
      </header>
      <section className="stats-grid">
        {stats.map(([label, value]) => (
          <article className="stat-card" key={label}>
            <span>{label}</span>
            <strong>{value}</strong>
          </article>
        ))}
      </section>
    </main>
  );
}
