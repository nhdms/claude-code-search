"use client";

import useSWR from "swr";
import fetcher, { Stats, SeriesPoint, Project, ToolCount } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  ResponsiveContainer, LineChart, Line, XAxis, YAxis, Tooltip, CartesianGrid,
  BarChart, Bar, PieChart, Pie, Cell, Legend,
} from "recharts";

const ROLE_COLORS: Record<string, string> = {
  user: "var(--color-chart-1)",
  assistant: "var(--color-chart-2)",
};

export default function Page() {
  const { data: stats } = useSWR<Stats>("/api/stats", fetcher);
  const { data: series } = useSWR<SeriesPoint[]>("/api/timeseries?bucket=day", fetcher);
  const { data: projects } = useSWR<Project[]>("/api/projects", fetcher);
  const { data: tools } = useSWR<ToolCount[]>("/api/tools", fetcher);

  const embeddedPct = stats && stats.chunks
    ? `${stats.embedded} (${((stats.embedded / stats.chunks) * 100).toFixed(0)}%)`
    : "—";

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Stat label="Sessions" value={stats?.sessions} />
        <Stat label="Messages" value={stats?.messages} />
        <Stat label="Chunks" value={stats?.chunks} />
        <Stat label="Embedded" value={embeddedPct} />
      </div>

      <Card>
        <CardHeader><CardTitle>Messages per day</CardTitle></CardHeader>
        <CardContent>
          <div className="h-72">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={series ?? []}>
                <CartesianGrid stroke="var(--color-border)" />
                <XAxis dataKey="bucket" stroke="var(--color-muted-foreground)" tick={{ fontSize: 11 }} />
                <YAxis stroke="var(--color-muted-foreground)" tick={{ fontSize: 11 }} />
                <Tooltip contentStyle={{ background: "var(--color-card)", border: "1px solid var(--color-border)" }} />
                <Legend />
                <Line type="monotone" dataKey="user" stroke="var(--color-chart-1)" dot={false} strokeWidth={1.5} />
                <Line type="monotone" dataKey="assistant" stroke="var(--color-chart-2)" dot={false} strokeWidth={1.5} />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card>
          <CardHeader><CardTitle>Top projects (by sessions)</CardTitle></CardHeader>
          <CardContent>
            <div className="h-80">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={(projects ?? []).slice(0, 12)} layout="vertical" margin={{ left: 60 }}>
                  <CartesianGrid stroke="var(--color-border)" />
                  <XAxis type="number" stroke="var(--color-muted-foreground)" tick={{ fontSize: 11 }} />
                  <YAxis dataKey="path" type="category" stroke="var(--color-muted-foreground)" tick={{ fontSize: 10 }} width={180} tickFormatter={(s: string) => s.split("/").slice(-2).join("/")} />
                  <Tooltip contentStyle={{ background: "var(--color-card)", border: "1px solid var(--color-border)" }} />
                  <Bar dataKey="sessions" fill="var(--color-chart-1)" />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Role distribution</CardTitle></CardHeader>
          <CardContent>
            <div className="h-80">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={Object.entries(stats?.roles ?? {}).map(([k, v]) => ({ name: k, value: v }))}
                    dataKey="value" nameKey="name" innerRadius={60} outerRadius={110} paddingAngle={2}
                  >
                    {Object.keys(stats?.roles ?? {}).map((k) => <Cell key={k} fill={ROLE_COLORS[k] ?? "var(--color-chart-3)"} />)}
                  </Pie>
                  <Tooltip contentStyle={{ background: "var(--color-card)", border: "1px solid var(--color-border)" }} />
                  <Legend />
                </PieChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader><CardTitle>Top tools</CardTitle></CardHeader>
        <CardContent>
          <div className="h-80">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={tools ?? []}>
                <CartesianGrid stroke="var(--color-border)" />
                <XAxis dataKey="name" stroke="var(--color-muted-foreground)" tick={{ fontSize: 10 }} angle={-30} textAnchor="end" height={70} />
                <YAxis stroke="var(--color-muted-foreground)" tick={{ fontSize: 11 }} />
                <Tooltip contentStyle={{ background: "var(--color-card)", border: "1px solid var(--color-border)" }} />
                <Bar dataKey="count" fill="var(--color-chart-3)" />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number | string | undefined }) {
  return (
    <Card>
      <CardContent className="p-5">
        <div className="text-xs uppercase tracking-wider text-muted-foreground">{label}</div>
        <div className="text-2xl font-semibold mt-1 tabular-nums">{value ?? "—"}</div>
      </CardContent>
    </Card>
  );
}
