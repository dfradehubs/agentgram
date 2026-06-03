import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// We need to test the internal fetchApi + ApiError logic.
// Since they are not directly exported, we test through exported functions
// that use them (e.g. getAgents, getSessions, deleteSession).

// Mock fetch globally
const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

// Prevent location redirect on 401
const originalLocation = window.location;
beforeEach(() => {
  Object.defineProperty(window, "location", {
    writable: true,
    value: { ...originalLocation, href: "", pathname: "/chat" },
  });
});

afterEach(() => {
  vi.resetAllMocks();
  Object.defineProperty(window, "location", {
    writable: true,
    value: originalLocation,
  });
});

// Import after mocking
import { getAgents, getSessions, deleteSession, getMe } from "../api";

function jsonResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function errorResponse(status: number, body?: unknown): Response {
  const init: ResponseInit = { status };
  if (body) {
    return new Response(JSON.stringify(body), {
      ...init,
      headers: { "Content-Type": "application/json" },
    });
  }
  return new Response(null, init);
}

describe("API client", () => {
  describe("successful requests", () => {
    it("getAgents parses agent list", async () => {
      const agents = [{ id: "a1", name: "Agent 1" }];
      mockFetch.mockResolvedValueOnce(jsonResponse({ agents }));

      const result = await getAgents();

      expect(result).toEqual(agents);
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/agents"),
        expect.objectContaining({
          headers: expect.objectContaining({
            "Content-Type": "application/json",
          }),
        })
      );
    });

    it("getSessions returns empty array when sessions is null", async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ sessions: null }));

      const result = await getSessions("agent-1");

      expect(result).toEqual([]);
    });

    it("deleteSession sends DELETE request", async () => {
      mockFetch.mockResolvedValueOnce(
        new Response(null, {
          status: 204,
          headers: { "content-length": "0" },
        })
      );

      await expect(deleteSession("a1", "s1")).resolves.toBeUndefined();
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/agents/a1/sessions/s1"),
        expect.objectContaining({ method: "DELETE" })
      );
    });
  });

  describe("error handling", () => {
    it("throws ApiError with status and message on non-ok response", async () => {
      mockFetch.mockResolvedValueOnce(
        errorResponse(500, { error: "Internal server error" })
      );

      await expect(getMe()).rejects.toMatchObject({
        name: "ApiError",
        message: "Internal server error",
        status: 500,
      });
    });

    it("throws ApiError with fallback message when body has no error field", async () => {
      mockFetch.mockResolvedValueOnce(errorResponse(502, {}));

      await expect(getMe()).rejects.toThrow("HTTP error 502");
    });

    it("throws ApiError when response body is not valid JSON", async () => {
      mockFetch.mockResolvedValueOnce(
        new Response("not json", { status: 500 })
      );

      await expect(getMe()).rejects.toThrow("HTTP error 500");
    });

    it("redirects to /login on 401", async () => {
      mockFetch.mockResolvedValueOnce(errorResponse(401, {}));

      await expect(getAgents()).rejects.toThrow("Session expired");
      expect(window.location.href).toBe("/login");
    });

    it("does not redirect on 401 if already on /login", async () => {
      window.location.pathname = "/login";
      mockFetch.mockResolvedValueOnce(errorResponse(401, {}));

      await expect(getAgents()).rejects.toThrow("Session expired");
      // href should not have been set to /login again
      expect(window.location.href).toBe("");
    });
  });
});
