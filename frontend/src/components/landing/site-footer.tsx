import Link from "next/link";
import { ZonyxLockup } from "./brand-logo";
import { LINKS } from "./config";

export function SiteFooter() {
  return (
    <footer className="lp-footer">
      <div className="lp-footer-top">
        <Link href="/" aria-label="Zonyx home"><ZonyxLockup /></Link>
        <nav aria-label="Footer">
          <Link href={LINKS.app}>App</Link>
          <Link href={LINKS.trading}>Trading</Link>
          <Link href={LINKS.research}>Research</Link>
          <a href={LINKS.docs} target="_blank" rel="noreferrer">Docs</a>
          <a href={LINKS.methodology} target="_blank" rel="noreferrer">Methodology</a>
        </nav>
      </div>
      <p className="lp-disclaimer">
        Zonyx provides market analysis, not financial advice. Scores are ranking scores, not probabilities. Nothing
        here is a guarantee of any outcome.
      </p>
    </footer>
  );
}
