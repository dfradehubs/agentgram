"use client";

import { Chat } from "../chat/Chat";

export function ChatArea() {
  return (
    <main id="chat-main" className="flex min-h-0 min-w-0 flex-1 flex-col" aria-label="Chat">
      <Chat />
    </main>
  );
}
