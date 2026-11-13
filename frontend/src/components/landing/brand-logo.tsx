import type { CSSProperties } from "react";
import { BRAND_ICONS } from "./brand-icons";
import type { LogoEntry } from "./config";

/** Monochrome brand mark (fills with currentColor). Falls back to a monogram when no mark is bundled. */
export function BrandLogo({ entry, className }: { entry: LogoEntry; className?: string }) {
  const icon = entry.icon ? BRAND_ICONS[entry.icon] : undefined;
  const style = { "--brand": icon?.hover ?? "#f5efe2" } as CSSProperties;
  if (!icon) {
    return (
      <span className={`lp-monogram ${className ?? ""}`} style={style} aria-hidden="true">
        {entry.name.slice(0, 1)}
      </span>
    );
  }
  return (
    <svg className={className} style={style} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path d={icon.path} />
    </svg>
  );
}

export function ZonyxLockup() {
  return (
    <span className="lp-lockup">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img src="/images/sightline-mark.svg" alt="" width={30} height={30} loading="lazy" />
      <span>Zonyx</span>
    </span>
  );
}
