import * as React from "react";
import { cn } from "@/lib/utils";

/** Status pill. The variants are the only states the panel distinguishes, so a
 *  screen cannot invent a colour with no meaning. */
const badgeVariants = {
  neutral: "border-border bg-muted text-muted-foreground",
  primary: "border-primary/25 bg-primary/10 text-primary",
  success: "border-success/30 bg-success/10 text-success",
  warning: "border-warning/35 bg-warning/12 text-warning",
  destructive: "border-destructive/30 bg-destructive/10 text-destructive",
  outline: "border-border text-foreground",
} as const;

export type BadgeVariant = keyof typeof badgeVariants;

function Badge({
  className,
  variant = "neutral",
  ...props
}: React.ComponentProps<"span"> & { variant?: BadgeVariant }) {
  return (
    <span
      data-slot="badge"
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        badgeVariants[variant],
        className,
      )}
      {...props}
    />
  );
}

export { Badge };
