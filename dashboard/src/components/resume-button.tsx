"use client";
import { useState } from "react";
import { Terminal, Loader2, Copy } from "lucide-react";
import { Button } from "@/components/ui/button";
import { toast } from "@/components/ui/sonner";
import { resumeCommand } from "@/lib/api";

type Sess = { id: string; cwd?: string; project?: string };

export function ResumeButton({
  session,
  variant = "outline",
  size = "sm",
  label = "Resume",
}: {
  session: Sess;
  variant?: "outline" | "ghost" | "secondary";
  size?: "sm" | "default" | "icon";
  label?: string;
}) {
  const [busy, setBusy] = useState(false);

  async function run(e: React.MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    setBusy(true);
    try {
      const r = await fetch("/api/resume", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          session_id: session.id,
          cwd: session.cwd ?? "",
          project: session.project ?? "",
        }),
      });
      if (!r.ok) throw new Error((await r.json()).error || `HTTP ${r.status}`);
      const j = await r.json();
      toast.success(`Opened in ${j.terminal}`, { description: j.command, duration: 4000 });
    } catch (err) {
      try {
        await navigator.clipboard.writeText(resumeCommand(session));
        toast.message("Couldn't open terminal — copied to clipboard", {
          description: String(err),
          duration: 5000,
        });
      } catch {
        toast.error("Open failed", { description: String(err) });
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <Button variant={variant} size={size} onClick={run} disabled={busy} title={`Resume session ${session.id}`}>
      {busy ? <Loader2 className="animate-spin" /> : <Terminal />}
      {size !== "icon" && <span>{busy ? "Opening…" : label}</span>}
    </Button>
  );
}

export function CopyResumeButton({ session }: { session: Sess }) {
  async function copy(e: React.MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    const cmd = resumeCommand(session);
    try {
      await navigator.clipboard.writeText(cmd);
      toast.success("Copied resume command", { description: cmd, duration: 3500 });
    } catch {
      toast.error("Copy failed");
    }
  }
  return (
    <Button variant="ghost" size="icon" onClick={copy} title="Copy resume command">
      <Copy />
    </Button>
  );
}

export function ResumeActions({ session }: { session: Sess }) {
  return (
    <div className="flex items-center gap-1 justify-end">
      <ResumeButton session={session} />
      <CopyResumeButton session={session} />
    </div>
  );
}
