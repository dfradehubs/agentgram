import { toJpeg } from "html-to-image";
import { jsPDF } from "jspdf";
import { reportMetric } from "@/lib/telemetry";
import { getT } from "@/lib/i18n";

// A4 dimensions in mm
const A4_W = 210;
const A4_H = 297;
const MARGIN = 6;
const HEADER_H = 3;
const FOOTER_H = 6;
const CONTENT_W = A4_W - MARGIN * 2;
const CONTENT_H = A4_H - MARGIN * 2 - HEADER_H - FOOTER_H;

const JPEG_QUALITY = 0.92;
const PIXEL_RATIO = 2;

// Max content pixels per capture chunk. At PIXEL_RATIO=2 this produces
// images of ~16,000px height, safely within browser canvas limits.
// Each chunk covers ~7 PDF pages, so a 34-page PDF needs only ~5 captures.
const MAX_CHUNK_PX = 8000;

function waitForPaint(): Promise<void> {
  return new Promise<void>((resolve) =>
    requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
  );
}

interface Overlay {
  element: HTMLDivElement;
  setProgress: (fraction: number) => void;
}

/**
 * Fixed-position overlay covering the chat area with a spinner and progress bar.
 * Uses position:fixed so it stays visible even when scrollContainer is expanded.
 */
function createOverlay(locale: string, container: HTMLElement): Overlay {
  const t = getT(locale as "es" | "en");
  const isDark = document.documentElement.classList.contains("dark");
  const rect = container.getBoundingClientRect();

  const overlay = document.createElement("div");
  Object.assign(overlay.style, {
    position: "fixed",
    top: `${rect.top}px`,
    left: `${rect.left}px`,
    width: `${rect.width}px`,
    bottom: "0",
    zIndex: "50",
    backgroundColor: isDark ? "#09090b" : "#ffffff",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    flexDirection: "column",
    gap: "12px",
  });

  const circle = document.createElement("div");
  Object.assign(circle.style, {
    width: "32px",
    height: "32px",
    border: `3px solid ${isDark ? "#27272a" : "#e5e7eb"}`,
    borderTopColor: "#6366f1",
    borderRadius: "50%",
    animation: "pdf-spin 0.8s linear infinite",
  });

  const label = document.createElement("div");
  Object.assign(label.style, {
    fontSize: "14px",
    color: isDark ? "#a1a1aa" : "#6b7280",
    fontFamily: "system-ui, sans-serif",
  });
  label.textContent = t("chat.exportingPDF");

  // Progress bar
  const barTrack = document.createElement("div");
  Object.assign(barTrack.style, {
    width: "200px",
    height: "4px",
    borderRadius: "2px",
    backgroundColor: isDark ? "#27272a" : "#e5e7eb",
    overflow: "hidden",
  });

  const barFill = document.createElement("div");
  Object.assign(barFill.style, {
    width: "0%",
    height: "100%",
    borderRadius: "2px",
    backgroundColor: "#6366f1",
    transition: "width 0.3s ease",
  });
  barTrack.appendChild(barFill);

  const style = document.createElement("style");
  style.textContent = "@keyframes pdf-spin{to{transform:rotate(360deg)}}";

  overlay.appendChild(style);
  overlay.appendChild(circle);
  overlay.appendChild(label);
  overlay.appendChild(barTrack);
  document.body.appendChild(overlay);

  return {
    element: overlay,
    setProgress: (fraction: number) => {
      barFill.style.width = `${Math.round(fraction * 100)}%`;
    },
  };
}

/**
 * Export the chat as a visual PDF snapshot.
 *
 * Captures in chunks (~7 pages each) to avoid browser canvas size limits
 * while minimizing the number of expensive toJpeg calls.
 *
 * Shows a fixed overlay with spinner over the chat area.
 * In dark mode, protects sidebar/header by scoping .dark to siblings.
 */
export async function exportChatToPDF(
  scrollContainer: HTMLDivElement,
  agentName: string,
  sessionTitle?: string,
): Promise<void> {
  const fullHeight = scrollContainer.scrollHeight;
  const locale = document.documentElement.lang || "es";

  // Save original styles
  const origScroll = {
    overflow: scrollContainer.style.overflow,
    height: scrollContainer.style.height,
    maxHeight: scrollContainer.style.maxHeight,
    flex: scrollContainer.style.flex,
    minHeight: scrollContainer.style.minHeight,
    position: scrollContainer.style.position,
  };

  // Show fixed overlay with spinner + progress bar (covers only the chat area viewport)
  const { element: overlay, setProgress } = createOverlay(locale, scrollContainer);
  await waitForPaint();

  // Block page scroll while capturing
  const origHtmlOverflow = document.documentElement.style.overflow;
  const origBodyOverflow = document.body.style.overflow;
  document.documentElement.style.overflow = "hidden";
  document.body.style.overflow = "hidden";

  // Now safe to manipulate — overlay hides everything underneath
  const wasDark = document.documentElement.classList.contains("dark");
  let darkRestored = false;

  // Protect sidebar/header: wrap siblings in <div class="dark" style="display:contents">
  // so they remain descendants of .dark when we remove it from <html>.
  // display:contents makes the wrapper invisible to layout.
  // Only needed when in dark mode.
  const darkWrappers: { wrapper: HTMLElement; sibling: HTMLElement; parent: HTMLElement }[] = [];
  if (wasDark) {
    let node: Element | null = scrollContainer;
    while (node && node !== document.body) {
      const parent: HTMLElement | null = node.parentElement;
      if (parent) {
        for (const sibling of Array.from(parent.children)) {
          if (sibling !== node && sibling instanceof HTMLElement) {
            const wrapper = document.createElement("div");
            wrapper.style.display = "contents";
            wrapper.classList.add("dark");
            parent.insertBefore(wrapper, sibling);
            wrapper.appendChild(sibling);
            darkWrappers.push({ wrapper, sibling, parent });
          }
        }
      }
      node = parent;
    }
    document.documentElement.classList.remove("dark");
  }

  // Track clip wrapper so we can clean up in finally
  let clipWrapper: HTMLDivElement | null = null;
  let contentParent: HTMLElement | null = null;

  try {
    // Expand scroll container so all content is rendered
    scrollContainer.style.overflow = "visible";
    scrollContainer.style.height = `${fullHeight}px`;
    scrollContainer.style.maxHeight = "none";
    scrollContainer.style.flex = "none";
    scrollContainer.style.minHeight = "auto";

    // Locate the inner content div
    const contentDiv =
      scrollContainer.querySelector<HTMLDivElement>("[data-pdf-content]");
    if (!contentDiv) {
      throw new Error("Could not find [data-pdf-content] element");
    }

    // Lock width and remove auto-centering for a tight capture
    const origContentStyles = {
      maxWidth: contentDiv.style.maxWidth,
      width: contentDiv.style.width,
      margin: contentDiv.style.margin,
      position: contentDiv.style.position,
      top: contentDiv.style.top,
    };
    const actualWidth = contentDiv.offsetWidth;
    contentDiv.style.width = `${actualWidth}px`;
    contentDiv.style.maxWidth = "none";
    contentDiv.style.margin = "0";

    await document.fonts.ready;
    await waitForPaint();

    // Measure content height after layout stabilizes
    const contentHeight = contentDiv.scrollHeight;

    // Page dimensions: how many content pixels fit in one PDF page
    const mmPerPx = CONTENT_W / actualWidth;
    const pageHeightPx = CONTENT_H / mmPerPx;
    const totalPages = Math.ceil(contentHeight / pageHeightPx);
    const pageCount = Math.max(totalPages, 1);

    // How many pages fit in one capture chunk (within canvas limits)
    const pagesPerChunk = Math.max(1, Math.floor(MAX_CHUNK_PX / pageHeightPx));

    // Create a clipping wrapper for chunked capture
    clipWrapper = document.createElement("div");
    Object.assign(clipWrapper.style, {
      overflow: "hidden",
      width: `${actualWidth}px`,
      position: "relative",
    });
    contentParent = contentDiv.parentElement!;
    contentParent.insertBefore(clipWrapper, contentDiv);
    clipWrapper.appendChild(contentDiv);
    contentDiv.style.position = "relative";

    const filterFn = (node: HTMLElement) => {
      if (node.classList?.contains("opacity-0")) return false;
      if (node.getAttribute?.("data-pdf-exclude") === "true") return false;
      if (node === overlay) return false;
      return true;
    };

    const pdf = new jsPDF({ orientation: "portrait", unit: "mm", format: "a4" });
    const title = sessionTitle || agentName;

    // Capture in chunks, then slice each chunk into pages via canvas
    const sliceCanvas = document.createElement("canvas");
    const ctx = sliceCanvas.getContext("2d")!;
    const totalChunks = Math.ceil(pageCount / pagesPerChunk);
    let chunkIdx = 0;

    for (let startPage = 0; startPage < pageCount; startPage += pagesPerChunk) {
      const endPage = Math.min(startPage + pagesPerChunk, pageCount);
      const chunkStartY = startPage * pageHeightPx;
      const chunkEndY = Math.min(endPage * pageHeightPx, contentHeight);
      const chunkH = chunkEndY - chunkStartY;

      // Position clip wrapper for this chunk
      clipWrapper.style.height = `${Math.ceil(chunkH)}px`;
      contentDiv.style.top = `-${chunkStartY}px`;

      await waitForPaint();

      const chunkDataUrl = await toJpeg(clipWrapper, {
        pixelRatio: PIXEL_RATIO,
        quality: JPEG_QUALITY,
        backgroundColor: "#ffffff",
        filter: filterFn,
      });

      const chunkImg = await loadImage(chunkDataUrl);
      chunkIdx++;
      setProgress(chunkIdx / totalChunks);
      const imgW = chunkImg.width;

      // Slice chunk into individual PDF pages
      sliceCanvas.width = imgW;

      for (let page = startPage; page < endPage; page++) {
        if (page > 0) pdf.addPage();

        // Indigo header bar
        pdf.setFillColor(99, 102, 241);
        pdf.rect(0, 0, A4_W, HEADER_H, "F");

        const pageOffsetInChunk = (page - startPage) * pageHeightPx;
        const sliceH = Math.min(pageHeightPx, contentHeight - page * pageHeightPx);

        const imgOffsetY = Math.round(pageOffsetInChunk * PIXEL_RATIO);
        const imgSliceH = Math.ceil(sliceH * PIXEL_RATIO);

        sliceCanvas.height = imgSliceH;
        ctx.drawImage(chunkImg, 0, imgOffsetY, imgW, imgSliceH, 0, 0, imgW, imgSliceH);
        const sliceUrl = sliceCanvas.toDataURL("image/jpeg", JPEG_QUALITY);

        const destH = sliceH * mmPerPx;
        pdf.addImage(sliceUrl, "JPEG", MARGIN, MARGIN + HEADER_H, CONTENT_W, destH);

        // Footer
        pdf.setFontSize(7);
        pdf.setTextColor(161, 161, 170);
        const footerY = A4_H - MARGIN;
        pdf.text(title, MARGIN, footerY);
        pdf.text(`${page + 1} / ${pageCount}`, A4_W - MARGIN, footerY, { align: "right" });
      }
    }

    // Restore DOM: move contentDiv back out of clip wrapper
    contentParent.insertBefore(contentDiv, clipWrapper);
    clipWrapper.remove();
    clipWrapper = null;

    // Restore contentDiv styles
    contentDiv.style.maxWidth = origContentStyles.maxWidth;
    contentDiv.style.width = origContentStyles.width;
    contentDiv.style.margin = origContentStyles.margin;
    contentDiv.style.position = origContentStyles.position;
    contentDiv.style.top = origContentStyles.top;

    // Restore scroll container layout
    Object.assign(scrollContainer.style, origScroll);

    // Restore theme before removing overlay
    if (wasDark) {
      document.documentElement.classList.add("dark");
      for (const { wrapper, sibling, parent } of darkWrappers) {
        parent.insertBefore(sibling, wrapper);
        wrapper.remove();
      }
      darkRestored = true;
    }

    // Save PDF
    const safeName = agentName.toLowerCase().replace(/\s+/g, "-");
    const filename = `chat-${safeName}-${new Date().toISOString().slice(0, 10)}.pdf`;
    pdf.save(filename);

    reportMetric({ name: "pdf_export", labels: { agent_id: safeName }, value: 1 });
  } finally {
    // Safety restore in case of error

    // Move contentDiv back if still inside clip wrapper
    if (clipWrapper && contentParent) {
      const contentDiv = clipWrapper.querySelector<HTMLDivElement>("[data-pdf-content]");
      if (contentDiv) {
        contentParent.insertBefore(contentDiv, clipWrapper);
        contentDiv.style.maxWidth = "";
        contentDiv.style.width = "";
        contentDiv.style.margin = "";
        contentDiv.style.position = "";
        contentDiv.style.top = "";
      }
      clipWrapper.remove();
    }

    const contentDiv =
      scrollContainer.querySelector<HTMLDivElement>("[data-pdf-content]");
    if (contentDiv) {
      contentDiv.style.maxWidth = "";
      contentDiv.style.width = "";
      contentDiv.style.margin = "";
      contentDiv.style.position = "";
      contentDiv.style.top = "";
    }

    Object.assign(scrollContainer.style, origScroll);

    if (wasDark && !darkRestored) {
      document.documentElement.classList.add("dark");
      for (const { wrapper, sibling, parent } of darkWrappers) {
        if (wrapper.parentElement) {
          parent.insertBefore(sibling, wrapper);
          wrapper.remove();
        }
      }
    }

    // Restore scroll
    document.documentElement.style.overflow = origHtmlOverflow;
    document.body.style.overflow = origBodyOverflow;

    overlay.remove();
  }
}

function loadImage(dataUrl: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = reject;
    img.src = dataUrl;
  });
}
