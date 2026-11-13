const STEPS = [
  { n: "01", title: "Collect", text: "Market and on-chain data is validated and normalized before anything is scored." },
  { n: "02", title: "Analyze", text: "Structure, liquidity and derivatives are scored separately for bullish and bearish evidence." },
  { n: "03", title: "Decide", text: "You see the evidence, the coverage and exactly what would invalidate the setup." },
];

export function HowItWorks() {
  return (
    <section className="lp-section" aria-labelledby="lp-how-h">
      <header className="lp-head">
        <span className="lp-label">How it works</span>
        <h2 id="lp-how-h">From raw data to a setup you can check.</h2>
      </header>
      <ol className="lp-steps">
        {STEPS.map((s) => (
          <li key={s.n} className="lp-step">
            <span className="lp-step-n">{s.n}</span>
            <h3>{s.title}</h3>
            <p>{s.text}</p>
          </li>
        ))}
      </ol>
    </section>
  );
}
