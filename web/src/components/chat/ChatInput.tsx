"use client";

import React from "react";
import { useT } from "@/lib/i18n";
import { AttachmentPreview } from "./AttachmentPreview";
import { Button } from "@/components/ui/button";
import {
  ArrowUp,
  Bot,
  Paperclip,
  Square,
} from "lucide-react";
import type { Attachment } from "@/lib/types";
import {
  applyMention,
  filterMentionOptions,
  findActiveMention,
  type MentionOption,
} from "@/lib/mentions";

const ACCEPTED_TYPES = "image/*,.pdf,.txt,.csv,.json";

interface ChatInputProps {
  // Mode
  isMCP: boolean;
  isInputDisabled: boolean;
  isLoading: boolean;
  // Input state
  input: string;
  setInput: (val: string) => void;
  mentionOptions?: MentionOption[];
  showMentionHint?: boolean;
  // Attachments
  pendingAttachments: Attachment[];
  onRemoveAttachment: (idx: number) => void;
  onFileSelect: (files: FileList | null) => void;
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
  isInputDisabled,
  isLoading,
  input,
  setInput,
  mentionOptions = [],
  showMentionHint = false,
  pendingAttachments,
  onRemoveAttachment,
  onFileSelect,
  onSend,
  onStop,
  widthCls,
  inputRef,
  fileInputRef,
}: ChatInputProps) {
  const t = useT();
  const [isFocused, setIsFocused] = React.useState(false);
  const [caret, setCaret] = React.useState(0);
  const [activeMentionIndex, setActiveMentionIndex] = React.useState(0);
  const [dismissedMention, setDismissedMention] = React.useState<string | null>(null);

  const activeMention = React.useMemo(
    () => mentionOptions.length > 0 ? findActiveMention(input, caret) : null,
    [input, caret, mentionOptions.length],
  );
  const filteredMentionOptions = React.useMemo(
    () => activeMention ? filterMentionOptions(mentionOptions, activeMention.query) : [],
    [activeMention, mentionOptions],
  );
  const mentionKey = activeMention
    ? `${activeMention.start}:${activeMention.end}:${activeMention.query}`
    : null;
  const isMentionListOpen = isFocused
    && !isInputDisabled
    && !!activeMention
    && filteredMentionOptions.length > 0
    && dismissedMention !== mentionKey;

  React.useEffect(() => {
    setActiveMentionIndex(0);
  }, [activeMention?.query, mentionOptions]);

  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInput(e.target.value);
    setCaret(e.target.selectionStart);
    setDismissedMention(null);
    const textarea = e.target;
    textarea.style.height = "auto";
    textarea.style.height = Math.min(textarea.scrollHeight, 200) + "px";
  };

  const selectMention = (option: MentionOption) => {
    if (!activeMention) return;
    const applied = applyMention(input, activeMention, option.id);
    setInput(applied.value);
    setCaret(applied.caret);
    setDismissedMention(null);
    requestAnimationFrame(() => {
      const textarea = inputRef.current;
      if (!textarea) return;
      textarea.focus();
      textarea.setSelectionRange(applied.caret, applied.caret);
      textarea.style.height = "auto";
      textarea.style.height = Math.min(textarea.scrollHeight, 200) + "px";
    });
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.nativeEvent.isComposing) return;
    if (isMentionListOpen) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        const direction = e.key === "ArrowDown" ? 1 : -1;
        setActiveMentionIndex((index) =>
          (index + direction + filteredMentionOptions.length) % filteredMentionOptions.length
        );
        return;
      }
      if ((e.key === "Enter" && !e.shiftKey) || e.key === "Tab") {
        e.preventDefault();
        selectMention(filteredMentionOptions[activeMentionIndex]);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setDismissedMention(mentionKey);
        return;
      }
    }
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
          <AttachmentPreview
            attachments={pendingAttachments}
            onRemove={onRemoveAttachment}
          />
          {isMentionListOpen && (
            <div
              id="chat-mention-suggestions"
              role="listbox"
              aria-label={t("chat.mentionSuggestions")}
              className="absolute bottom-full left-10 right-4 z-20 mb-2 max-h-64 overflow-y-auto rounded-lg border bg-popover p-1 text-popover-foreground shadow-lg"
            >
              {filteredMentionOptions.map((option, index) => (
                <button
                  key={option.id}
                  id={`chat-mention-option-${index}`}
                  type="button"
                  role="option"
                  aria-selected={index === activeMentionIndex}
                  onMouseEnter={() => setActiveMentionIndex(index)}
                  onMouseDown={(event) => {
                    event.preventDefault();
                  }}
                  onClick={() => selectMention(option)}
                  className={`flex w-full items-start gap-2 rounded-md px-2.5 py-2 text-left ${
                    index === activeMentionIndex ? "bg-accent text-accent-foreground" : "hover:bg-accent/60"
                  }`}
                >
                  <Bot className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1">
                    <span className="flex items-baseline gap-2">
                      <span className="truncate text-sm font-medium">{option.label}</span>
                      <span className="truncate text-xs text-muted-foreground">@{option.id}</span>
                    </span>
                    {option.description && (
                      <span className="block truncate text-xs text-muted-foreground">{option.description}</span>
                    )}
                  </span>
                </button>
              ))}
            </div>
          )}
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
              onFocus={(event) => {
                setIsFocused(true);
                setCaret(event.currentTarget.selectionStart);
              }}
              onBlur={() => setIsFocused(false)}
              onClick={(event) => {
                setCaret(event.currentTarget.selectionStart);
                setDismissedMention(null);
              }}
              onSelect={(event) => {
                setCaret(event.currentTarget.selectionStart);
                setDismissedMention(null);
              }}
              onPaste={handlePaste}
              placeholder={
                isInputDisabled
                  ? (isMCP ? t("mcp.disconnectedPlaceholder") : t("chat.placeholderGithub"))
                  : t("chat.placeholder")
              }
              disabled={isInputDisabled}
              rows={1}
              role="combobox"
              aria-autocomplete="list"
              aria-expanded={isMentionListOpen}
              aria-controls={isMentionListOpen ? "chat-mention-suggestions" : undefined}
              aria-activedescendant={isMentionListOpen ? `chat-mention-option-${activeMentionIndex}` : undefined}
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
          {showMentionHint && <> · {t("chat.mentionHint")}</>}
        </span>
      </div>
    </div>
  );
});
