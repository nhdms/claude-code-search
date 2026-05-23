"use client";
import { useState } from "react";
import { RefreshCcw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { toast } from "@/components/ui/sonner";
import { Job } from "@/lib/api";

export function ReindexButton({ sessionId, onDone }: { sessionId: string; onDone?: () => void }) {
  const [busy, setBusy] = useState(false);

  async function run() {
    setBusy(true);
    try {
      const r = await fetch(`/api/sessions/${sessionId}/reindex`, { method: "POST" });
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      const job = (await r.json()) as Job;
      toast.message("Reindex started", { description: job.id });
      const final = await pollJob(job.id);
      if (final.status === "completed") {
        toast.success("Reindex complete");
        onDone?.();
      } else {
        toast.error("Reindex failed", { description: final.error });
      }
    } catch (e) {
      toast.error("Reindex failed", { description: String(e) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Button variant="outline" size="sm" onClick={run} disabled={busy}>
      <RefreshCcw className={busy ? "animate-spin" : ""} />
      {busy ? "Reindexing…" : "Reindex"}
    </Button>
  );
}

async function pollJob(id: string): Promise<Job> {
  while (true) {
    const r = await fetch(`/api/jobs/${id}`);
    const j = (await r.json()) as Job;
    if (j.status !== "running") return j;
    await new Promise((res) => setTimeout(res, 1500));
  }
}
