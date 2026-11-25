import type { Metadata } from "next";
import "./globals.css";
import { OfflineRegister } from "./offline-register";
export const metadata: Metadata = {
  title: "MEKU-In | MEKU SOLUTIONS",
  description: "Bar inventory, sales and cash control.",
};
export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body><OfflineRegister />{children}</body></html>;
}
