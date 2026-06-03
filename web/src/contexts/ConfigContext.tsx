"use client";

import { createContext, useContext, useEffect, useState } from "react";

export interface LLMModel {
  id: string;
  name: string;
  provider: string;
  default?: boolean;
}

interface AppConfig {
  features: {
    summarizer_enabled: boolean;
  };
  available_models: LLMModel[];
}

const defaultConfig: AppConfig = {
  features: {
    summarizer_enabled: false,
  },
  available_models: [],
};

const ConfigContext = createContext<AppConfig>(defaultConfig);

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "";

export function ConfigProvider({ children }: { children: React.ReactNode }) {
  const [config, setConfig] = useState<AppConfig>(defaultConfig);

  useEffect(() => {
    fetch(`${API_BASE_URL}/api/config`, { credentials: "include" })
      .then((res) => (res.ok ? res.json() : defaultConfig))
      .then((data) => setConfig({ ...defaultConfig, ...data }))
      .catch(() => {
        // Config endpoint unavailable - use defaults
      });
  }, []);

  return (
    <ConfigContext.Provider value={config}>{children}</ConfigContext.Provider>
  );
}

export function useConfig() {
  return useContext(ConfigContext);
}
