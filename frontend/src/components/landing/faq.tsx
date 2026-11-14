const FAQS = [
  {
    q: "Does Zonyx hold my funds?",
    a: "No. Swaps are signed in your own wallet and Zonyx never sees your keys or seed phrase.",
  },
  {
    q: "Is a score a prediction?",
    a: "No. It is a ranking score that orders setups by evidence, not a probability of any outcome.",
  },
  {
    q: "Does Zonyx trade for me?",
    a: "No. Trading Intelligence is read-only and never places a trade or a swap.",
  },
  {
    q: "What is an Order Block here?",
    a: "A zone detected by our own published rules. It does not prove that any institution placed orders there.",
  },
  {
    q: "What happens when data is missing?",
    a: "The signal is hidden or marked as partial. Zonyx does not fill gaps with estimates.",
  },
];

export function Faq() {
  return (
    <section className="lp-section lp-faq-wrap" aria-labelledby="lp-faq-h">
      <header className="lp-head">
        <h2 id="lp-faq-h">FAQ</h2>
      </header>
      <div className="lp-faq">
        {FAQS.map((f) => (
          <details key={f.q}>
            <summary>
              <span>{f.q}</span>
              <span className="lp-faq-icon" aria-hidden="true" />
            </summary>
            <p>{f.a}</p>
          </details>
        ))}
      </div>
    </section>
  );
}
