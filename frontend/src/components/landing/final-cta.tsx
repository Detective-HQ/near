import Link from "next/link";
import { LINKS } from "./config";
import { LeafSprout } from "./foliage";

export function FinalCta() {
  return (
    <section className="lp-section lp-cta" aria-labelledby="lp-cta-h">
      <div className="lp-cta-glow-valley" aria-hidden="true">
        <span className="cta-flora cta-flora-1" />
        <span className="cta-flora cta-flora-2" />
        <span className="cta-flora cta-flora-3" />
      </div>
      <span className="lp-label"><LeafSprout className="lp-label-sprout" />Enter the garden</span>
      <h2 id="lp-cta-h">
        <span>One interface.</span> <span>Multiple chains.</span> <span>Verifiable finance.</span>
      </h2>
      <p>See the market the way the data shows it.</p>
      <div className="lp-cta-actions">
        <Link className="lp-btn lp-btn-primary" href={LINKS.app}>Launch app</Link>
        <a className="lp-btn lp-btn-secondary" href={LINKS.methodology} target="_blank" rel="noreferrer">
          Read the methodology
        </a>
      </div>
    </section>
  );
}
