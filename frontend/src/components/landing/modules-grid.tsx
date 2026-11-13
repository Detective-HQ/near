import Link from "next/link";
import { MODULES } from "./config";

export function ModulesGrid() {
  return (
    <section className="lp-section" aria-labelledby="lp-mod-h">
      <header className="lp-head">
        <h2 id="lp-mod-h">One workspace, nine tools.</h2>
      </header>
      <ul className="lp-modules">
        {MODULES.map((m, i) => (
          <li key={m.name}>
            <Link className="lp-module" href={m.href}>
              <span className="lp-module-n">{String(i + 1).padStart(2, "0")}</span>
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
