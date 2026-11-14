import { BrandLogo } from "./brand-logo";
import { PROVIDERS, STACK } from "./config";
import type { LogoEntry } from "./config";

function Group({ title, entries }: { title: string; entries: LogoEntry[] }) {
  const shown = entries.filter((e) => e.enabled);
  if (shown.length === 0) return null;
  return (
    <div className="lp-tech-group">
      <h3 className="lp-label">{title}</h3>
      <ul className="lp-tech">
        {shown.map((e) => (
          <li key={e.name} className="lp-tech-cell">
            <BrandLogo entry={e} className="lp-tech-logo" />
            <span>{e.name}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function TechGrid() {
  return (
    <section className="lp-section" aria-labelledby="lp-tech-h">
      <header className="lp-head">
        <h2 id="lp-tech-h">Built on tools you already trust.</h2>
      </header>
      <Group title="Stack" entries={STACK} />
      <Group title="Data & routing" entries={PROVIDERS} />
    </section>
  );
}
