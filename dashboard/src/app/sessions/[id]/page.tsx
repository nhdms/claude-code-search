"use client";

import useSWR from "swr";
import { use } from "react";
import fetcher, { SessionDetail } from "@/lib/api";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { ResumeButton } from "@/components/resume-button";
import { ReindexButton } from "@/components/reindex-button";

export default function SessionPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { data, mutate } = useSWR<SessionDetail>(`/api/sessions/${id}`, fetcher);

  if (!data) return <div className="text-muted-foreground">Loading…</div>;
  const { session, messages } = data;

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4">
          <div className="space-y-1">
            <div className="text-xs uppercase text-muted-foreground tracking-wider">Session</div>
            <div className="font-mono text-sm break-all">{session.id}</div>
            <div className="text-sm">{session.project || session.cwd}</div>
            <div className="text-muted-foreground text-xs">
              {session.started_at?.slice(0, 19).replace("T", " ")} → {session.ended_at?.slice(0, 19).replace("T", " ")}
            </div>
          </div>
          <div className="flex gap-2">
            <ResumeButton session={session} />
            <ReindexButton sessionId={session.id} onDone={() => mutate()} />
          </div>
        </CardHeader>
      </Card>

      <div className="space-y-3">
        {(messages ?? []).map((m) => (
          <Card key={m.uuid}>
            <CardContent className="p-4">
              <div className="flex justify-between text-xs text-muted-foreground mb-2 items-center">
                <div className="flex items-center gap-2">
                  <Badge variant="outline" className={m.role === "user" ? "role-user" : "role-assistant"}>
                    {m.role}
                  </Badge>
                  <Badge variant="secondary">{m.kind}</Badge>
                  {m.tool_name && <Badge variant="outline">{m.tool_name}</Badge>}
                </div>
                <span className="tabular-nums">{m.ts}</span>
              </div>
              {m.text && <pre className="whitespace-pre-wrap text-sm font-mono leading-relaxed">{m.text}</pre>}
              {m.kind === "tool_use" && m.tool_input && (
                <details className="mt-2">
                  <summary className="text-xs text-muted-foreground cursor-pointer">tool input</summary>
                  <pre className="whitespace-pre-wrap text-xs text-muted-foreground mt-1">{m.tool_input.slice(0, 4000)}</pre>
                </details>
              )}
              {m.kind === "tool_result" && m.tool_output && (
                <details className="mt-2">
                  <summary className="text-xs text-muted-foreground cursor-pointer">tool output</summary>
                  <pre className="whitespace-pre-wrap text-xs text-muted-foreground mt-1">{m.tool_output.slice(0, 4000)}</pre>
                </details>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
