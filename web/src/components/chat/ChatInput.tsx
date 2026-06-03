"use client";

import React from "react";
import { useT } from "@/lib/i18n";
import { AttachmentPreview } from "./AttachmentPreview";
import { AgentSelector } from "./AgentSelector";
import { Button } from "@/components/ui/button";
import {
  ArrowUp,
  Paperclip,
  Square,
} from "lucide-react";
import type { Attachment } from "@/lib/types";

const ACCEPTED_TYPES = "image/*,.pdf,.txt,.csv,.json";

interface ChatInputProps {
  // Mode
  isMCP: boolean;
  isMultiAgent: boolean;
  isInputDisabled: boolean;
  isLoading: boolean;
  // Input state
  input: string;
  setInput: (val: string) => void;
  // Attachments
  pendingAttachments: Attachment[];
  onRemoveAttachment: (idx: number) => void;
  onFileSelect: (files: FileList | null) => void;
  // Multi-agent
  multiAgentIds: string[];
  selectedTargetAgentIds: string[];
  onToggleTargetAgent: (agentId: string) => void;
  // Actions
  onSend: () => void;
  onStop: () => void;
  // Width
  widthCls: string;
  // Refs
  inputRef: React.RefObject<HTMLTextAreaElement | null>;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
}

export const ChatInput = React.memo(function ChatInput({
  isMCP,
  isMultiAgent,
  isInputDisabled,
  isLoading,
  input,
  setInput,
  pendingAttachments,
  onRemoveAttachment,
  onFileSelect,
  multiAgentIds,
  selectedTargetAgentIds,
  onToggleTargetAgent,
  onSend,
  onStop,
  widthCls,
  inputRef,
  fileInputRef,
}: ChatInputProps) {
  const t = useT();

  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInput(e.target.value);
    const textarea = e.target;
    textarea.style.height = "auto";
    textarea.style.height = Math.min(textarea.scrollHeight, 200) + "px";
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      onSend();
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    onFileSelect(e.dataTransfer.files);
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
  };

  const handlePaste = (e: React.ClipboardEvent) => {
    const files = e.clipboardData.files;
    if (files.length > 0) {
      e.preventDefault();
      onFileSelect(files);
    }
  };

  return (
    <div className="safe-area-bottom border-t bg-background px-4 py-4">
      <div className={`mx-auto ${widthCls}`}>
        <div
          className="relative rounded-xl border bg-card px-4 py-3 shadow-sm focus-within:border-ring focus-within:ring-1 focus-within:ring-ring"
          onDrop={handleDrop}
          onDragOver={handleDragOver}
        >
          {isMultiAgent && (
            <AgentSelector
              agentIds={multiAgentIds}
              selectedAgentIds={selectedTargetAgentIds}
              onToggle={onToggleTargetAgent}
            />
          )}
          <AttachmentPreview
            attachments={pendingAttachments}
            onRemove={onRemoveAttachment}
          />
          <div className="flex items-center gap-2">
            <input
              ref={fileInputRef}
              type="file"
              multiple
              accept={ACCEPTED_TYPES}
              className="hidden"
              onChange={(e) => onFileSelect(e.target.files)}
            />
            <Button
              variant="ghost"
              size="icon"
              onClick={() => fileInputRef.current?.click()}
              disabled={isInputDisabled}
              className="h-8 w-8 shrink-0 rounded-lg"
              aria-label={t("chat.attachFile")}
            >
              <Paperclip className="h-4 w-4" />
            </Button>
            <textarea
              ref={inputRef}
              value={input}
              onChange={handleInputChange}
              onKeyDown={handleKeyDown}
              onPaste={handlePaste}
              placeholder={
                isInputDisabled
                  ? (isMCP ? t("mcp.disconnectedPlaceholder") : t("chat.placeholderGithub"))
                  : t("chat.placeholder")
              }
              disabled={isInputDisabled}
              rows={1}
              aria-label={t("chat.message")}
              className="max-h-[200px] min-h-[40px] max-w-full flex-1 resize-none overflow-hidden bg-transparent py-2 text-sm leading-6 outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50"
            />
            {isLoading ? (
              <Button
                variant="ghost"
                size="icon"
                onClick={onStop}
                className="h-8 w-8 shrink-0 rounded-lg"
                aria-label={t("chat.stopResponse")}
              >
                <Square className="h-4 w-4" />
              </Button>
            ) : (
              <Button
                size="icon"
                onClick={onSend}
                disabled={isInputDisabled || (!input.trim() && pendingAttachments.length === 0)}
                className="h-8 w-8 shrink-0 rounded-lg"
                aria-label={t("chat.sendMessage")}
              >
                <ArrowUp className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>
        <span className="p-2 block text-center text-[10px] text-muted-foreground">
          {t("chat.inputHint")}
        </span>
      </div>
    </div>
  );
});
