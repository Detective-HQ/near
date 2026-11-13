const RIBBON_1 = ["Non-custodial", "Read-only signals", "No automated trading", "Your keys stay yours"];
const RIBBON_2 = ["Source on every metric", "Rankings, not probabilities", "Unavailable is not bearish", "Versioned models"];

function Band({ phrases, variant }: { phrases: string[]; variant: "fill" | "line" }) {
  // Repeated enough times to cover wide screens; the track moves by exactly half its width.
  const run = (hidden: boolean) => (
    <span className="lp-ribbon-run" aria-hidden={hidden || undefined}>
      {[0, 1].flatMap((k) =>
        phrases.map((p) => (
          <span key={`${k}-${p}`} className="lp-ribbon-item">
            {p}
            <span className="lp-ribbon-mark" aria-hidden="true">✦</span>
          </span>
        )),
      )}
    </span>
  );
  return (
    <div className={`lp-ribbon lp-ribbon-${variant}`}>
      <div className="lp-ribbon-track">
        {run(false)}
        {run(true)}
      </div>
    </div>
  );
}

export function Ribbons() {
  return (
    <section className="lp-ribbons" aria-label="Commitments">
      <Band phrases={RIBBON_1} variant="fill" />
      <Band phrases={RIBBON_2} variant="line" />
    </section>
  );
}
