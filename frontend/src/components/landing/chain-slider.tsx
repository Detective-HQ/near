import { BrandLogo } from "./brand-logo";
import { CHAINS } from "./config";

export function ChainSlider() {
  const chains = CHAINS.filter((c) => c.enabled);
  if (chains.length === 0) return null;
  // The list is rendered twice so the loop is seamless; the copy is hidden from assistive tech.
  const row = (hidden: boolean) => (
    <ul className="lp-marquee-row" aria-hidden={hidden || undefined}>
      {chains.map((c) => (
        <li key={c.name} className="lp-chain">
          <BrandLogo entry={c} className="lp-chain-logo" />
          <span>{c.name}</span>
        </li>
      ))}
    </ul>
  );
  return (
    <section className="lp-section lp-chains" aria-labelledby="lp-chains-label">
      <p className="lp-label lp-center" id="lp-chains-label">One interface across</p>
      <div className="lp-marquee">
        <div className="lp-marquee-track">
          {row(false)}
          {row(true)}
        </div>
      </div>
    </section>
  );
}
