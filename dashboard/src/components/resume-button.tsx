"use client";
import { Copy, Terminal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { toast } from "@/components/ui/sonner";
import { resumeCommand } from "@/lib/api";

export function ResumeButton({
  session,
  variant = "outline",
  size = "sm",
  label = "Resume",
}: {
  session: { id: string; cwd?: string; project?: string };
  variant?: "outline" | "ghost" | "secondary";
  size?: "sm" | "default" | "icon";
  label?: string;
}) {
  const cmd = resumeCommand(session);
  const copy = async (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(cmd);
      toast.success("Copied resume command", { description: cmd, duration: 3500 });
    } catch {
      toast.error("Copy failed");
    }
  };
  return (
    <Button variant={variant} size={size} onClick={copy} title={cmd}>
      {size === "icon" ? <Copy /> : <><Terminal /> {label}</>}
    </Button>
  );
}
