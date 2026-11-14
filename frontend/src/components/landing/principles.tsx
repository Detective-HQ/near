import { FlowerBloom } from "./foliage";

const PRINCIPLES = [
  { title: "Non-custodial", text: "Signing happens in your wallet." },
  { title: "Read-only analysis", text: "Signals never place a trade." },
  { title: "Evidence first", text: "Every score shows its coverage." },
  { title: "Honest gaps", text: "Missing data is shown as missing." },
];

export function Principles() {
  return (
    <section className="lp-section lp-principles" aria-label="Principles">
      <ul>
        {PRINCIPLES.map((p) => (
          <li key={p.title}>
            <div className="lp-principle-title">
              <FlowerBloom className="lp-principle-flower" />
              <h3>{p.title}</h3>
            </div>
            <p>{p.text}</p>
          </li>
        ))}
      </ul>
    </section>
  );
}
