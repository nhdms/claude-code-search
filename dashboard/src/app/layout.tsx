import "./globals.css";
import Link from "next/link";
import type { ReactNode } from "react";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";

export const metadata = { title: "claude-search dashboard" };

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" className="dark">
      <body className="min-h-screen bg-background text-foreground">
        <TooltipProvider delayDuration={150}>
        <header className="border-b border-border sticky top-0 z-40 backdrop-blur bg-background/80">
          <div className="max-w-[1400px] mx-auto px-6 py-3 flex items-center gap-6">
            <span className="font-semibold tracking-tight">claude-search</span>
            <nav className="flex gap-5 text-sm text-muted-foreground">
              <Link href="/" className="hover:text-foreground transition">Overview</Link>
              <Link href="/sessions" className="hover:text-foreground transition">Sessions &amp; Search</Link>
              <Link href="/settings" className="hover:text-foreground transition">Settings</Link>
            </nav>
          </div>
        </header>
        <main className="px-6 py-6 max-w-[1400px] mx-auto">{children}</main>
        </TooltipProvider>
        <Toaster position="bottom-right" />
      </body>
    </html>
  );
}
