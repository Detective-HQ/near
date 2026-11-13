import Link from "next/link";
import { LINKS } from "./config";

export function FinalCta() {
  return (
    <section className="lp-section lp-cta" aria-labelledby="lp-cta-h">
      <h2 id="lp-cta-h">One interface. Multiple chains. Verifiable finance.</h2>
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
