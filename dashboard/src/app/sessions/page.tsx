"use client";

import { useEffect, useState } from "react";
import useSWR from "swr";
import Link from "next/link";
import fetcher, { Project, SearchResp, SessionList } from "@/lib/api";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ResumeActions } from "@/components/resume-button";

const PAGE = 30;

export default function SessionsPage() {
  const [project, setProject] = useState("");
  const [q, setQ] = useState("");
  const [role, setRole] = useState("");
  const [since, setSince] = useState("");
  const [offset, setOffset] = useState(0);
  const [vector, setVector] = useState(true);
  const [tab, setTab] = useState<"sessions" | "messages">("sessions");

  const { data: projects } = useSWR<Project[]>("/api/projects", fetcher);

  const sessionParams = new URLSearchParams();
  if (project) sessionParams.set("project", project);
  if (q) sessionParams.set("q", q);
  if (since) sessionParams.set("since", since);
  sessionParams.set("limit", String(PAGE));
  sessionParams.set("offset", String(offset));

  const { data: list, isLoading: listLoading } = useSWR<SessionList>(`/api/sessions?${sessionParams}`, fetcher);

  const [searchResult, setSearchResult] = useState<SearchResp | null>(null);
  const [searching, setSearching] = useState(false);

  async function runMessageSearch() {
    if (!q.trim()) { setSearchResult(null); return; }
    setSearching(true);
    const p = new URLSearchParams({ q, limit: "30" });
    if (role) p.set("role", role);
    if (project) p.set("project", project);
    if (!vector) p.set("vector", "false");
    if (since) p.set("since", since);
    try {
      const r = await fetch(`/api/search?${p}`);
      const j = await r.json();
      setSearchResult(j);
    } finally {
      setSearching(false);
    }
  }

  useEffect(() => {
    if (tab === "messages" && q.trim()) runMessageSearch();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab]);

  function submit(e: React.FormEvent) {
    e.preventDefault();
    setOffset(0);
    if (tab === "messages") runMessageSearch();
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="p-4">
          <form onSubmit={submit} className="flex flex-wrap items-end gap-3">
            <div className="flex-1 min-w-[260px] flex flex-col gap-1.5">
              <Label>Search</Label>
              <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="text, keywords, or project name…" />
            </div>
            <Field label="Project">
              <Select value={project || "__all"} onValueChange={(v) => { setProject(v === "__all" ? "" : v); setOffset(0); }}>
                <SelectTrigger className="w-56"><SelectValue placeholder="(all)" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all">(all)</SelectItem>
                  {(projects ?? []).map((p) => (
                    <SelectItem key={p.path} value={p.path}>{p.path} ({p.sessions})</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label="Role">
              <Select value={role || "__any"} onValueChange={(v) => setRole(v === "__any" ? "" : v)}>
                <SelectTrigger className="w-28"><SelectValue placeholder="any" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="__any">any</SelectItem>
                  <SelectItem value="user">user</SelectItem>
                  <SelectItem value="assistant">assistant</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label="Since">
              <Input type="date" className="w-40" value={since} onChange={(e) => setSince(e.target.value)} />
            </Field>
            <label className="flex items-center gap-2 text-sm pb-1.5">
              <input type="checkbox" checked={vector} onChange={(e) => setVector(e.target.checked)} className="size-4 accent-foreground" />
              vector
            </label>
            <Button type="submit" disabled={searching}>{searching ? "…" : "Search"}</Button>
            <Button type="button" variant="outline" onClick={() => { setQ(""); setRole(""); setProject(""); setSince(""); setOffset(0); setSearchResult(null); }}>Reset</Button>
          </form>
        </CardContent>
      </Card>

      <Tabs value={tab} onValueChange={(v) => setTab(v as typeof tab)}>
        <TabsList>
          <TabsTrigger value="sessions">Sessions {list ? `(${list.total})` : ""}</TabsTrigger>
          <TabsTrigger value="messages">Messages {searchResult ? `(${searchResult.hits.length})` : ""}</TabsTrigger>
        </TabsList>

        <TabsContent value="sessions" className="space-y-3">
          <Card>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Started</TableHead>
                    <TableHead>Title / Project</TableHead>
                    <TableHead>Messages</TableHead>
                    <TableHead>Session</TableHead>
                    <TableHead className="text-right pr-4">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {listLoading && <TableRow><TableCell className="text-muted-foreground" colSpan={5}>Loading…</TableCell></TableRow>}
                  {(list?.items ?? []).map((s) => (
                    <TableRow key={s.id}>
                      <TableCell className="whitespace-nowrap text-muted-foreground">{s.started_at?.slice(0, 19).replace("T", " ")}</TableCell>
                      <TableCell className="max-w-[460px]">
                        <div className="flex flex-col gap-0.5 min-w-0">
                          <span className="truncate text-sm">{s.title || <span className="text-muted-foreground italic">(untitled)</span>}</span>
                          <span className="truncate text-xs text-muted-foreground font-mono">{s.project || s.cwd || "—"}</span>
                        </div>
                      </TableCell>
                      <TableCell className="tabular-nums">{s.messages}</TableCell>
                      <TableCell>
                        <Link className="text-primary hover:underline font-mono text-xs" href={`/sessions/${s.id}`}>{s.id.slice(0, 8)}…</Link>
                      </TableCell>
                      <TableCell className="text-right pr-4">
                        <ResumeActions session={s} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          <div className="flex items-center justify-between text-sm text-muted-foreground">
            <span>{list ? `${offset + 1}–${Math.min(offset + PAGE, list.total)} of ${list.total}` : ""}</span>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE))}>Prev</Button>
              <Button variant="outline" size="sm" disabled={!list || offset + PAGE >= list.total} onClick={() => setOffset(offset + PAGE)}>Next</Button>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="messages" className="space-y-3">
          {searchResult?.projects && searchResult.projects.length > 0 && (
            <Card>
              <CardContent className="p-4 flex flex-wrap gap-2">
                <span className="text-xs uppercase text-muted-foreground tracking-wider mr-1 self-center">Projects:</span>
                {searchResult.projects.map((p) => (
                  <Badge key={p.path} variant="outline" className="cursor-pointer hover:bg-accent" onClick={() => { setProject(p.path); setTab("sessions"); setOffset(0); }}>
                    {p.path} · {p.sessions}
                  </Badge>
                ))}
              </CardContent>
            </Card>
          )}

          {searchResult && (
            <div className="text-xs text-muted-foreground">
              {searchResult.hits.length} hits · {searchResult.vector ? "hybrid FTS + vector" : "FTS only"}
            </div>
          )}

          {!searchResult && !searching && (
            <Card><CardContent className="p-6 text-sm text-muted-foreground">Enter a query and press Search.</CardContent></Card>
          )}

          {(searchResult?.hits ?? []).map((h) => (
            <Link key={h.message_uuid} href={`/sessions/${h.session_id}`}>
              <Card className="hover:bg-accent/30 transition-colors mb-3">
                <CardContent className="p-4">
                  <div className="flex justify-between text-xs text-muted-foreground mb-2 items-center">
                    <div className="flex items-center gap-2">
                      <Badge variant="outline" className={h.role === "user" ? "role-user" : "role-assistant"}>{h.role}</Badge>
                      <Badge variant="secondary">score {h.score.toFixed(4)}</Badge>
                      {h.fts_rank > 0 && <Badge variant="outline">fts #{h.fts_rank}</Badge>}
                      {h.vec_rank > 0 && <Badge variant="outline">vec #{h.vec_rank}</Badge>}
                    </div>
                    <span className="tabular-nums">{h.ts}</span>
                  </div>
                  <div className="text-xs text-muted-foreground mb-2 truncate">{h.project}</div>
                  <pre className="whitespace-pre-wrap text-sm font-mono leading-relaxed">
                    {h.text.slice(0, 600)}{h.text.length > 600 ? "…" : ""}
                  </pre>
                </CardContent>
              </Card>
            </Link>
          ))}
        </TabsContent>
      </Tabs>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label>{label}</Label>
      {children}
    </div>
  );
}
