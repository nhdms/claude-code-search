const fetcher = (url: string) => fetch(url).then((r) => r.json());
export default fetcher;

export type Stats = {
  sessions: number;
  messages: number;
  chunks: number;
  embedded: number;
  first_ts: string;
  last_ts: string;
  roles: Record<string, number>;
  vector_ready: boolean;
  projects_dir: string;
};

export type SeriesPoint = { bucket: string; user: number; assistant: number; total: number };
export type Project = { path: string; sessions: number };
export type ToolCount = { name: string; count: number };

export type SessionRow = {
  id: string;
  project: string;
  cwd: string;
  started_at: string;
  ended_at: string;
  messages: number;
};

export type SessionList = { total: number; items: SessionRow[]; limit: number; offset: number };

export type Message = {
  uuid: string;
  role: string;
  kind: string;
  ts: string;
  text: string;
  tool_name?: string;
  tool_input?: string;
  tool_output?: string;
};

export type SessionDetail = { session: SessionRow; messages: Message[] };

export type Hit = {
  chunk_id: number;
  message_uuid: string;
  session_id: string;
  role: string;
  ts: string;
  project: string;
  text: string;
  fts_rank: number;
  vec_rank: number;
  score: number;
};

export type SearchResp = { hits: Hit[]; vector: boolean; projects: Project[] };

export type Config = {
  openai_key_masked: string;
  projects_dir: string;
  embed_model: string;
  embed_dim: number;
  base_url: string;
  db_dim: number;
};

export type Job = {
  id: string;
  kind: string;
  status: "running" | "completed" | "failed";
  message: string;
  progress: number;
  stats?: unknown;
  error?: string;
  started_at: string;
  ended_at?: string;
};

export function resumeCommand(session: { id: string; cwd?: string; project?: string }) {
  const dir = session.cwd || session.project || "~";
  return `cd "${dir}" && claude --resume ${session.id}`;
}
