"use client";

import { useState } from "react";
import { Header } from "@/components/layout/Header";
import { Sidebar } from "@/components/layout/Sidebar";
import { ChatArea } from "@/components/layout/ChatArea";
import { ErrorBoundary } from "@/components/ErrorBoundary";

export default function Home() {
  const [sidebarOpen, setSidebarOpen] = useState(true);

  return (
    <div className="flex h-dvh w-full flex-col bg-background">
      <a href="#chat-main" className="skip-to-content">
        Ir al contenido principal
      </a>
      <Header
        onToggleSidebar={() => setSidebarOpen(!sidebarOpen)}
        sidebarOpen={sidebarOpen}
      />
      <div className="flex min-h-0 flex-1">
        <ErrorBoundary>
          <Sidebar
            isOpen={sidebarOpen}
            onClose={() => setSidebarOpen(false)}
          />
        </ErrorBoundary>
        <ErrorBoundary>
          <ChatArea />
        </ErrorBoundary>
      </div>
    </div>
  );
}
