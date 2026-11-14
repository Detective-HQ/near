import Link from "next/link";
import { MODULES } from "./config";
import { CardFernCorner, LeafSprout } from "./foliage";

export function ModulesGrid() {
  return (
    <section className="lp-section" aria-labelledby="lp-mod-h">
      <header className="lp-head">
        <span className="lp-label"><LeafSprout className="lp-label-sprout" />Modular ecosystem</span>
        <h2 id="lp-mod-h">One workspace, nine tools.</h2>
      </header>
      <ul className="lp-modules">
        {MODULES.map((m, i) => (
          <li key={m.name}>
            <Link className="lp-module" href={m.href}>
              <CardFernCorner className="module-fern-corner" />
              <span className="lp-module-n">
                <LeafSprout className="lp-label-sprout" />
                {String(i + 1).padStart(2, "0")}
              </span>
              <h3>{m.name}</h3>
              <p>{m.text}</p>
              <span className="lp-module-arrow" aria-hidden="true">→</span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
