import * as React from "react";
import { cn } from "@/lib/utils";

export const Sidebar = React.forwardRef<
  HTMLDivElement,
  React.ComponentProps<"div">
>(({ className, ...props }, ref) => {
  return (
    <div
      ref={ref}
      className={cn(
        "flex h-full w-64 flex-col bg-sidebar text-sidebar-foreground border-r",
        className
      )}
      {...props}
    />
  );
});
Sidebar.displayName = "Sidebar";

export const SidebarContent = React.forwardRef<
  HTMLDivElement,
  React.ComponentProps<"div">
>(({ className, ...props }, ref) => {
  return (
    <div
      ref={ref}
      className={cn("flex flex-1 flex-col gap-2 overflow-auto", className)}
      {...props}
    />
  );
});
SidebarContent.displayName = "SidebarContent";
