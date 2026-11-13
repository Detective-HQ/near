import type { Metadata } from "next";
import type { ReactNode } from "react";
import { Geist } from "next/font/google";
import "./globals.css";

const geist = Geist({
  subsets: ["latin"]
});

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
      <body className={geist.className}>{children}</body>
    </html>
  );
}
