"use client";

import type { Attachment } from "@/lib/types";
import { FileText, X } from "lucide-react";

interface AttachmentPreviewProps {
  attachments: Attachment[];
  onRemove: (index: number) => void;
}

export function AttachmentPreview({ attachments, onRemove }: AttachmentPreviewProps) {
  if (attachments.length === 0) return null;

  return (
    <div className="flex flex-wrap gap-2 pb-2">
      {attachments.map((att, index) => {
        const isImage = att.content_type.startsWith("image/");
        return (
          <div
            key={index}
            className="group relative flex items-center gap-2 rounded-lg border bg-muted/50 px-3 py-2 text-xs"
          >
            {isImage ? (
              <img
                src={`data:${att.content_type};base64,${att.data}`}
                alt={att.filename}
                className="h-8 w-8 rounded object-cover"
              />
            ) : (
              <FileText className="h-4 w-4 text-muted-foreground" />
            )}
            <span className="max-w-[120px] truncate">{att.filename}</span>
            <button
              onClick={() => onRemove(index)}
              className="ml-1 rounded-full p-0.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            >
              <X className="h-3 w-3" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
