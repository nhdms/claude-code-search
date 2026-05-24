"use client";

import { useEffect, useState } from "react";
import useSWR from "swr";
import fetcher, { Config, Job } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { toast } from "@/components/ui/sonner";

type Preset = {
  id: string;
  label: string;
  provider: "openai" | "local";
  model: string;
  dim: number;
  base_url: string;
  hint: string;
};

const PRESETS: Preset[] = [
  { id: "openai-small", label: "OpenAI · text-embedding-3-small",  provider: "openai", model: "text-embedding-3-small",  dim: 1536, base_url: "openai",                          hint: "$0.02 / 1M tok · 1536d" },
  { id: "openai-large", label: "OpenAI · text-embedding-3-large",  provider: "openai", model: "text-embedding-3-large",  dim: 3072, base_url: "openai",                          hint: "$0.13 / 1M tok · 3072d" },
  { id: "ollama-nomic", label: "Ollama · nomic-embed-text",         provider: "local",  model: "nomic-embed-text",        dim: 768,  base_url: "http://localhost:11434/v1",        hint: "free local · 768d · pull: ollama pull nomic-embed-text" },
  { id: "ollama-mxbai", label: "Ollama · mxbai-embed-large",        provider: "local",  model: "mxbai-embed-large",       dim: 1024, base_url: "http://localhost:11434/v1",        hint: "free local · 1024d · pull: ollama pull mxbai-embed-large" },
  { id: "ollama-bge",   label: "Ollama · bge-m3",                   provider: "local",  model: "bge-m3",                  dim: 1024, base_url: "http://localhost:11434/v1",        hint: "free local · 1024d · pull: ollama pull bge-m3" },
  { id: "custom",       label: "Custom (OpenAI-compatible)",        provider: "local",  model: "",                        dim: 1536, base_url: "",                                hint: "any OpenAI-compatible /v1/embeddings endpoint" },
];

export default function SettingsPage() {
  const { data: cfg, mutate } = useSWR<Config>("/api/config", fetcher);
  const [key, setKey] = useState("");
  const [projectsDir, setProjectsDir] = useState("");
  const [presetId, setPresetId] = useState("openai-small");
  const [model, setModel] = useState("text-embedding-3-small");
  const [dim, setDim] = useState(1536);
  const [baseURL, setBaseURL] = useState("");
  const [resetVec, setResetVec] = useState(false);
  const [importPath, setImportPath] = useState("");
  const [toolMode, setToolMode] = useState<"none" | "small" | "all">("small");
  const [doEmbed, setDoEmbed] = useState(true);
  const [job, setJob] = useState<Job | null>(null);

  useEffect(() => {
    if (cfg?.projects_dir) {
      setProjectsDir((v) => v || cfg.projects_dir);
      setImportPath((v) => v || cfg.projects_dir);
    }
    if (cfg?.embed_model) setModel((v) => v || cfg.embed_model);
    if (cfg?.embed_dim) setDim((v) => v || cfg.embed_dim);
    if (cfg?.base_url != null) setBaseURL((v) => v || cfg.base_url);
  }, [cfg]);

  const preset = PRESETS.find((p) => p.id === presetId)!;

  function applyPreset(id: string) {
    setPresetId(id);
    const p = PRESETS.find((x) => x.id === id)!;
    if (id !== "custom") {
      setModel(p.model);
      setDim(p.dim);
      setBaseURL(p.base_url === "openai" ? "" : p.base_url);
    }
    if (cfg?.db_dim != null && cfg.db_dim !== p.dim) setResetVec(true);
  }

  async function saveConfig() {
    const body: Record<string, unknown> = {};
    if (key) body.openai_api_key = key;
    if (projectsDir) body.projects_dir = projectsDir;
    if (model) body.embed_model = model;
    if (dim) body.embed_dim = dim;
    body.base_url = baseURL || "openai";
    if (resetVec) body.reset_vectors = true;
    const r = await fetch("/api/config", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (!r.ok) { toast.error("Save failed"); return; }
    setKey("");
    setResetVec(false);
    toast.success("Saved");
    mutate();
  }

  async function runImport() {
    setJob(null);
    const r = await fetch("/api/import", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: importPath, tool_mode: toolMode, embed: doEmbed }),
    });
    if (!r.ok) { toast.error("Import failed to start"); return; }
    const j = (await r.json()) as Job;
    setJob(j);
    poll(j.id);
  }

  async function poll(id: string) {
    while (true) {
      const r = await fetch(`/api/jobs/${id}`);
      const j = (await r.json()) as Job;
      setJob(j);
      if (j.status !== "running") {
        if (j.status === "completed") toast.success("Import complete", { description: j.message });
        else toast.error("Import failed", { description: j.error });
        return;
      }
      await new Promise((res) => setTimeout(res, 1500));
    }
  }

  return (
    <div className="space-y-6 max-w-3xl">
      <Card>
        <CardHeader>
          <CardTitle>Embedding model</CardTitle>
          <CardDescription>OpenAI for best quality, or a local model via Ollama or any OpenAI-compatible endpoint. Dimension is locked in the DB — switching to a different-dim model resets all vectors.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-col gap-1.5">
            <Label>Preset</Label>
            <Select value={presetId} onValueChange={applyPreset}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {PRESETS.map((p) => <SelectItem key={p.id} value={p.id}>{p.label}</SelectItem>)}
              </SelectContent>
            </Select>
            <span className="text-xs text-muted-foreground">{preset.hint}</span>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div className="md:col-span-2 flex flex-col gap-1.5">
              <Label>Model</Label>
              <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="text-embedding-3-small" />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label>Dim</Label>
              <Input type="number" value={dim} onChange={(e) => setDim(Number(e.target.value) || 0)} />
            </div>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>Base URL <span className="lowercase normal-case font-normal text-muted-foreground/70">(blank = OpenAI)</span></Label>
            <Input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} placeholder="http://localhost:11434/v1" />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>API key {preset.provider === "local" && <span className="lowercase normal-case font-normal text-muted-foreground/70">(usually unused for local — leave blank)</span>}</Label>
            <Input type="password" value={key} onChange={(e) => setKey(e.target.value)} placeholder={cfg?.openai_key_masked || "sk-..."} />
            {cfg?.openai_key_masked && <span className="text-xs text-muted-foreground">current: <span className="font-mono">{cfg.openai_key_masked}</span></span>}
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>Projects dir</Label>
            <Input value={projectsDir} onChange={(e) => setProjectsDir(e.target.value)} placeholder="~/.claude/projects" />
          </div>
          {cfg && cfg.db_dim !== dim && (
            <div className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-xs space-y-2">
              <div>
                Current DB dim is <Badge variant="outline">{cfg.db_dim}</Badge>, selected dim is <Badge variant="outline">{dim}</Badge>.
              </div>
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={resetVec} onChange={(e) => setResetVec(e.target.checked)} className="size-4 accent-foreground" />
                Drop existing vectors and mark all chunks as pending (you&apos;ll need to re-embed)
              </label>
            </div>
          )}
          <div className="flex justify-end pt-1">
            <Button onClick={saveConfig}>Save</Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Import / re-import projects</CardTitle>
          <CardDescription>Point at any directory containing Claude Code projects. Existing sessions are de-duplicated by message UUID, so re-running is safe.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-col gap-1.5">
            <Label>Path</Label>
            <Input value={importPath} onChange={(e) => setImportPath(e.target.value)} placeholder="/Users/you/.claude/projects" />
          </div>
          <div className="flex gap-3 items-end flex-wrap">
            <div className="flex flex-col gap-1.5">
              <Label>Tool output</Label>
              <Select value={toolMode} onValueChange={(v) => setToolMode(v as typeof toolMode)}>
                <SelectTrigger className="w-40"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">none</SelectItem>
                  <SelectItem value="small">small (≤2KB)</SelectItem>
                  <SelectItem value="all">all</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <label className="flex items-center gap-2 text-sm pb-1.5">
              <input type="checkbox" checked={doEmbed} onChange={(e) => setDoEmbed(e.target.checked)} className="size-4 accent-foreground" />
              embed after import
            </label>
            <div className="flex-1" />
            <Button onClick={runImport} disabled={job?.status === "running"}>
              {job?.status === "running" ? "Running…" : "Start import"}
            </Button>
          </div>

          {job && (
            <div className="border rounded-md p-3 mt-2 space-y-2">
              <div className="flex justify-between text-xs items-center">
                <span className="font-mono text-muted-foreground">{job.id}</span>
                <Badge variant={job.status === "completed" ? "outline" : job.status === "failed" ? "destructive" : "secondary"}>
                  {job.status}
                </Badge>
              </div>
              <div className="h-1.5 bg-secondary rounded-full overflow-hidden">
                <div className="h-full bg-primary transition-all" style={{ width: `${Math.round((job.progress ?? 0) * 100)}%` }} />
              </div>
              <div className="text-xs text-muted-foreground">{job.message}</div>
              {job.error && <div className="text-xs text-destructive">{job.error}</div>}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Live sync</CardTitle>
          <CardDescription>How new conversations get into the index.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <p className="text-muted-foreground">
            <strong className="text-foreground">Recommended:</strong> install a Stop hook so every Claude Code session is auto-synced when it pauses. No daemon, incremental, idempotent.
          </p>
          <pre className="bg-secondary/30 border rounded-md p-3 text-xs overflow-x-auto">{`claude-search hook install
claude-search hook status
claude-search hook uninstall`}</pre>
          <p className="text-muted-foreground text-xs">
            Writes <code>~/.claude/settings.json</code> with the absolute path to this binary. Existing hooks &amp; permissions are preserved.
          </p>
          <p className="text-muted-foreground">
            Or run <code>claude-search watch</code> for sub-second mid-session indexing.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
