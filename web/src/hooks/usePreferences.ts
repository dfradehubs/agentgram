"use client";

import { useState, useCallback, useEffect } from "react";

export interface UserPreferences {
  theme: "light" | "dark" | "system";
  chatWidth: "normal" | "wide" | "full";
  locale: "es" | "en";
  showThinking: boolean;
}

const STORAGE_KEY = "agentgram-preferences";

const defaults: UserPreferences = {
  theme: "system",
  chatWidth: "wide",
  locale: "en",
  showThinking: true,
};

function loadPreferences(): UserPreferences {
  if (typeof window === "undefined") return defaults;
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return defaults;
    return { ...defaults, ...JSON.parse(stored) };
  } catch {
    return defaults;
  }
}

function savePreferences(prefs: UserPreferences) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
  } catch {
    // localStorage unavailable
  }
}

function applyTheme(theme: UserPreferences["theme"]) {
  const root = document.documentElement;
  if (
    theme === "dark" ||
    (theme === "system" &&
      window.matchMedia("(prefers-color-scheme: dark)").matches)
  ) {
    root.classList.add("dark");
  } else {
    root.classList.remove("dark");
  }
}

export function usePreferences() {
  const [preferences, setPreferences] = useState<UserPreferences>(() => {
    const loaded = loadPreferences();
    if (typeof window !== "undefined") {
      applyTheme(loaded.theme);
    }
    return loaded;
  });

  // Listen for system theme changes when theme is "system"
  useEffect(() => {
    if (preferences.theme !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = () => applyTheme("system");
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, [preferences.theme]);

  const updatePreference = useCallback(
    <K extends keyof UserPreferences>(key: K, value: UserPreferences[K]) => {
      setPreferences((prev) => {
        const next = { ...prev, [key]: value };
        savePreferences(next);
        if (key === "theme") {
          applyTheme(value as UserPreferences["theme"]);
        }
        if (key === "locale") {
          document.documentElement.lang = value as string;
        }
        return next;
      });
    },
    []
  );

  return { preferences, updatePreference };
}
