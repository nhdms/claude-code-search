"use client";
import { Toaster as Sonner, toast } from "sonner";

const Toaster = (props: React.ComponentProps<typeof Sonner>) => (
  <Sonner
    theme="dark"
    className="toaster group"
    toastOptions={{
      classNames: {
        toast: "group toast group-[.toaster]:bg-card group-[.toaster]:text-card-foreground group-[.toaster]:border-border group-[.toaster]:shadow-lg",
      },
    }}
    {...props}
  />
);

export { Toaster, toast };
