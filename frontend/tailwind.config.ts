import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        primary: {
          EFAULT: "hsl(var(--primary))",
          foreground: "hsl(var(--primary-foreground))",
          glow: "hsl(var(--primary-glow))",
        },
        surface: "#f9fafb",
        foreground: "#111827",
        muted: "#6b7280",
        background: "hsl(var(--background))",
      },
      boxShadow: {
        glow: "0 0 10px rgba(37,99,235,0.4)",
      },
    },
  },
  plugins: [],
} satisfies Config;
