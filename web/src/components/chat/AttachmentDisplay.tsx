"use client";

import type { Attachment } from "@/lib/types";
import { FileText, Image } from "lucide-react";

interface AttachmentDisplayProps {
  attachments: Attachment[];
}

export function AttachmentDisplay({ attachments }: AttachmentDisplayProps) {
  if (!attachments || attachments.length === 0) return null;

  return (
    <div className="mt-1 flex flex-wrap gap-1.5">
      {attachments.map((att, index) => {
        const isImage = att.content_type.startsWith("image/");
        return (
          <span
            key={index}
            className="inline-flex items-center gap-1 rounded-md bg-muted/50 px-2 py-0.5 text-xs text-muted-foreground"
          >
            {isImage ? (
              <Image className="h-3 w-3" aria-hidden="true" />
            ) : (
              <FileText className="h-3 w-3" />
            )}
            {att.filename}
          </span>
        );
      })}
    </div>
  );
}
