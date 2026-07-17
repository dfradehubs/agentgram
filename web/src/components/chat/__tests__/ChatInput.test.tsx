import React, { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ChatInput } from "../ChatInput";
import type { MentionOption } from "@/lib/mentions";

vi.mock("@/lib/i18n", () => ({ useT: () => (key: string) => key }));

const mentionOptions: MentionOption[] = [
  { id: "logs-agent", label: "Log Explorer", description: "Searches logs" },
  { id: "metrics-agent", label: "Metrics", description: "Reads dashboards" },
];

afterEach(cleanup);

function Harness({ options = mentionOptions, onSend = vi.fn() }: {
  options?: MentionOption[];
  onSend?: () => void;
}) {
  const [input, setInput] = useState("");
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  return (
    <ChatInput
      isMCP={false}
      isInputDisabled={false}
      isLoading={false}
      input={input}
      setInput={setInput}
      mentionOptions={options}
      showMentionHint
      pendingAttachments={[]}
      onRemoveAttachment={vi.fn()}
      onFileSelect={vi.fn()}
      onSend={onSend}
      onStop={vi.fn()}
      widthCls="max-w-xl"
      inputRef={inputRef}
      fileInputRef={fileInputRef}
    />
  );
}

describe("ChatInput mention autocomplete", () => {
  beforeEach(() => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
  });

  it("opens on @ and exposes accessible combobox semantics", () => {
    render(<Harness />);
    const textarea = screen.getByRole("combobox");
    fireEvent.focus(textarea);
    fireEvent.change(textarea, { target: { value: "@" } });

    expect(screen.getByRole("listbox", { name: "chat.mentionSuggestions" })).toBeInTheDocument();
    expect(screen.getAllByRole("option")).toHaveLength(2);
    expect(textarea).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText(/chat.mentionHint/)).toBeInTheDocument();
  });

  it("filters by display name", () => {
    render(<Harness />);
    const textarea = screen.getByRole("combobox");
    fireEvent.focus(textarea);
    fireEvent.change(textarea, { target: { value: "@expl" } });

    expect(screen.getByRole("option", { name: /Log Explorer/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /Metrics/ })).not.toBeInTheDocument();
  });

  it("uses arrows and Enter to insert the canonical id without sending", () => {
    const onSend = vi.fn();
    render(<Harness onSend={onSend} />);
    const textarea = screen.getByRole("combobox") as HTMLTextAreaElement;
    fireEvent.focus(textarea);
    fireEvent.change(textarea, { target: { value: "@" } });
    fireEvent.keyDown(textarea, { key: "ArrowDown" });
    fireEvent.keyDown(textarea, { key: "Enter" });

    expect(textarea.value).toBe("@metrics-agent ");
    expect(textarea.selectionStart).toBe(15);
    expect(onSend).not.toHaveBeenCalled();
  });

  it("inserts with the mouse and closes with Escape", () => {
    render(<Harness />);
    const textarea = screen.getByRole("combobox") as HTMLTextAreaElement;
    fireEvent.focus(textarea);
    fireEvent.change(textarea, { target: { value: "ask @met" } });
    const option = screen.getByRole("option", { name: /Metrics/ });
    fireEvent.mouseDown(option);
    fireEvent.click(option);
    expect(textarea.value).toBe("ask @metrics-agent ");

    fireEvent.change(textarea, { target: { value: "ask @" } });
    fireEvent.keyDown(textarea, { key: "Escape" });
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("keeps Enter as send when no suggestion is open", () => {
    const onSend = vi.fn();
    render(<Harness options={[]} onSend={onSend} />);
    const textarea = screen.getByRole("combobox");
    fireEvent.focus(textarea);
    fireEvent.change(textarea, { target: { value: "hello" } });
    fireEvent.keyDown(textarea, { key: "Enter" });
    expect(onSend).toHaveBeenCalledOnce();
  });
});
