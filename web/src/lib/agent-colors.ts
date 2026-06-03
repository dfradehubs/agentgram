// Stable color assignment for agents and MCP servers based on their ID.
// Each entity gets a consistent color that persists across renders and sessions.

interface EntityColor {
  // Tailwind classes for chat bubble border
  border: string;
  // Tailwind classes for chat bubble background
  bg: string;
  // CSS gradient for avatar
  avatarFrom: string;
  avatarTo: string;
  // Tailwind class for sidebar icon background
  iconBg: string;
  // Tailwind class for sidebar icon foreground
  iconFg: string;
}

// Curated palette of 10 elegant, subtle colors
const PALETTE: EntityColor[] = [
  {
    border: "border-blue-400/40 dark:border-blue-400/25",
    bg: "bg-blue-50/50 dark:bg-blue-950/20",
    avatarFrom: "#3b82f6",
    avatarTo: "#2563eb",
    iconBg: "bg-blue-500/10",
    iconFg: "text-blue-600 dark:text-blue-400",
  },
  {
    border: "border-emerald-400/40 dark:border-emerald-400/25",
    bg: "bg-emerald-50/50 dark:bg-emerald-950/20",
    avatarFrom: "#10b981",
    avatarTo: "#059669",
    iconBg: "bg-emerald-500/10",
    iconFg: "text-emerald-600 dark:text-emerald-400",
  },
  {
    border: "border-violet-400/40 dark:border-violet-400/25",
    bg: "bg-violet-50/50 dark:bg-violet-950/20",
    avatarFrom: "#8b5cf6",
    avatarTo: "#7c3aed",
    iconBg: "bg-violet-500/10",
    iconFg: "text-violet-600 dark:text-violet-400",
  },
  {
    border: "border-amber-400/40 dark:border-amber-400/25",
    bg: "bg-amber-50/50 dark:bg-amber-950/20",
    avatarFrom: "#f59e0b",
    avatarTo: "#d97706",
    iconBg: "bg-amber-500/10",
    iconFg: "text-amber-600 dark:text-amber-400",
  },
  {
    border: "border-rose-400/40 dark:border-rose-400/25",
    bg: "bg-rose-50/50 dark:bg-rose-950/20",
    avatarFrom: "#f43f5e",
    avatarTo: "#e11d48",
    iconBg: "bg-rose-500/10",
    iconFg: "text-rose-600 dark:text-rose-400",
  },
  {
    border: "border-cyan-400/40 dark:border-cyan-400/25",
    bg: "bg-cyan-50/50 dark:bg-cyan-950/20",
    avatarFrom: "#06b6d4",
    avatarTo: "#0891b2",
    iconBg: "bg-cyan-500/10",
    iconFg: "text-cyan-600 dark:text-cyan-400",
  },
  {
    border: "border-indigo-400/40 dark:border-indigo-400/25",
    bg: "bg-indigo-50/50 dark:bg-indigo-950/20",
    avatarFrom: "#6366f1",
    avatarTo: "#4f46e5",
    iconBg: "bg-indigo-500/10",
    iconFg: "text-indigo-600 dark:text-indigo-400",
  },
  {
    border: "border-teal-400/40 dark:border-teal-400/25",
    bg: "bg-teal-50/50 dark:bg-teal-950/20",
    avatarFrom: "#14b8a6",
    avatarTo: "#0d9488",
    iconBg: "bg-teal-500/10",
    iconFg: "text-teal-600 dark:text-teal-400",
  },
  {
    border: "border-orange-400/40 dark:border-orange-400/25",
    bg: "bg-orange-50/50 dark:bg-orange-950/20",
    avatarFrom: "#f97316",
    avatarTo: "#ea580c",
    iconBg: "bg-orange-500/10",
    iconFg: "text-orange-600 dark:text-orange-400",
  },
  {
    border: "border-pink-400/40 dark:border-pink-400/25",
    bg: "bg-pink-50/50 dark:bg-pink-950/20",
    avatarFrom: "#ec4899",
    avatarTo: "#db2777",
    iconBg: "bg-pink-500/10",
    iconFg: "text-pink-600 dark:text-pink-400",
  },
];

// Simple string hash (djb2) to get a stable index from an ID
function hashString(str: string): number {
  let hash = 5381;
  for (let i = 0; i < str.length; i++) {
    hash = ((hash << 5) + hash + str.charCodeAt(i)) | 0;
  }
  return Math.abs(hash);
}

export function getEntityColor(id: string): EntityColor {
  const idx = hashString(id) % PALETTE.length;
  return PALETTE[idx];
}
