import type { SVGProps } from "react";

/** Delicate botanical leaf sprout icon used beside section and card labels */
export function LeafSprout(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 16 16"
      fill="currentColor"
      width="13"
      height="13"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      <path d="M8 15V9.5C8 6.5 10 4 14 3c-.5 3-2 5.5-5 5.9V15H8Z" />
      <path d="M7 11.5C4.5 11 2.5 8.5 2 5c3.5-.2 5.5 1.5 6 4.5" opacity="0.75" />
    </svg>
  );
}

/** Delicate night flower bloom used on step badges and markers */
export function FlowerBloom(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 20 20"
      fill="currentColor"
      width="14"
      height="14"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      <circle cx="10" cy="10" r="2.2" />
      <path d="M10 2.5C9.2 4.5 9.2 5.8 10 7c.8-1.2.8-2.5 0-4.5Z" />
      <path d="M10 17.5C9.2 15.5 9.2 14.2 10 13c.8 1.2.8 2.5 0 4.5Z" />
      <path d="M2.5 10C4.5 9.2 5.8 9.2 7 10c-1.2.8-2.5.8-4.5 0Z" />
      <path d="M17.5 10C15.5 9.2 14.2 9.2 13 10c1.2.8 2.5.8 4.5 0Z" />
      <path d="M4.7 4.7C6.3 5.8 7.2 6.7 7.5 8.3 6.9 7.7 5.8 6.8 4.7 4.7Z" opacity="0.6" />
      <path d="M15.3 15.3C13.7 14.2 12.8 13.3 12.5 11.7c.6.6 1.7 1.5 2.8 3.6Z" opacity="0.6" />
      <path d="M15.3 4.7C14.2 6.3 13.3 7.2 11.7 7.5c.6-.6 1.5-1.7 3.6-2.8Z" opacity="0.6" />
      <path d="M4.7 15.3C5.8 13.7 6.7 12.8 8.3 12.5c-.6.6-1.5 1.7-3.6 2.8Z" opacity="0.6" />
    </svg>
  );
}

/** Delicate botanical fern frond / foliage corner silhouette for cards */
export function CardFernCorner({ className }: { className?: string }) {
  return (
    <svg
      className={`card-foliage-corner ${className ?? ""}`}
      viewBox="0 0 120 120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      focusable="false"
    >
      {/* Central curving vine */}
      <path
        d="M120 0C100 20 85 45 75 80C70 95 68 110 65 120"
        stroke="currentColor"
        strokeWidth="1.2"
        strokeLinecap="round"
      />
      {/* Delicate foliage fronds */}
      <path d="M108 8C96 11 88 8 82 2C86 12 94 16 108 8Z" fill="currentColor" fillOpacity="0.4" />
      <path d="M96 22C84 27 76 25 72 18C74 29 82 34 96 22Z" fill="currentColor" fillOpacity="0.5" />
      <path d="M86 38C74 44 66 43 62 36C63 47 71 52 86 38Z" fill="currentColor" fillOpacity="0.55" />
      <path d="M78 56C66 63 58 63 54 56C54 67 62 72 78 56Z" fill="currentColor" fillOpacity="0.6" />
      <path d="M72 75C61 82 54 83 50 77C49 87 56 92 72 75Z" fill="currentColor" fillOpacity="0.65" />
      <path d="M67 96C58 102 52 103 48 98C46 107 53 111 67 96Z" fill="currentColor" fillOpacity="0.7" />
      {/* Glowing floral pollen nodes sampled from the 3D flower valley */}
      <circle cx="88" cy="18" r="1.5" fill="var(--bloom-yellow, #ffcf3d)" opacity="0.8" />
      <circle cx="74" cy="35" r="1.5" fill="var(--bloom-pink, #f25aa5)" opacity="0.75" />
      <circle cx="64" cy="54" r="1.5" fill="var(--bloom-orange, #ff7a2e)" opacity="0.8" />
      <circle cx="56" cy="74" r="1.5" fill="var(--bloom-violet, #a67cff)" opacity="0.75" />
    </svg>
  );
}

/** Ambient glowing flora background component */
export function GardenAmbience() {
  return (
    <div className="lp-garden-ambience" aria-hidden="true">
      <div className="lp-garden-mesh" />
      <div className="lp-garden-canopy" />
      <div className="lp-garden-fireflies">
        <span className="fly fly-1" />
        <span className="fly fly-2" />
        <span className="fly fly-3" />
        <span className="fly fly-4" />
        <span className="fly fly-5" />
        <span className="fly fly-6" />
      </div>
    </div>
  );
}
