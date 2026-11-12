import type { Metadata } from "next";
import type { ReactNode } from "react";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "Universal DeFi Intelligence",
    template: "%s · Universal DeFi Intelligence"
  },
  description: "Cross-chain quote research and source-attributed NEAR ecosystem intelligence."
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
