import { describe, it, expect, vi } from "vitest";
import { uiPrefix, apiUrl, UI_DOOR } from "./apiPath";

describe("apiPath", () => {
  it("returns no prefix for the default root mount", () => {
    expect(uiPrefix("/ui/")).toBe("");
    expect(uiPrefix("/ui")).toBe("");
  });

  it("returns the prefix the proxy mounted the UI under", () => {
    expect(uiPrefix("/asubpath/llama-swap/ui/")).toBe("/asubpath/llama-swap");
    expect(uiPrefix("/asubpath/llama-swap/ui")).toBe("/asubpath/llama-swap");
    expect(uiPrefix("/llama-swap/ui/")).toBe("/llama-swap");
  });

  it("returns no prefix when the path is not the UI mount", () => {
    expect(uiPrefix("/")).toBe("");
    expect(uiPrefix("/ui/models/foo")).toBe("");
  });

  it("leaves paths unchanged outside the browser", () => {
    // Vitest runs in a Node environment: no window, rooted paths pass
    // through so cli/fetchBase.ts can resolve them against --base-url.
    expect(apiUrl("/api/mcp")).toBe("/api/mcp");
  });

  describe("in a browser", () => {
    const at = (pathname: string, path: string) => {
      vi.stubGlobal("window", { location: { pathname } });
      try {
        return apiUrl(path);
      } finally {
        vi.unstubAllGlobals();
      }
    };

    it("sends every request through the UI door", () => {
      // The door path must match uiPrefix in internal/server/auth.go.
      expect(UI_DOOR).toBe("/ui");
      // auth.ui is checked at /ui/, so the browser never needs an API key.
      expect(at("/ui/", "/api/version")).toBe("/ui/api/version");
      expect(at("/ui", "/v1/chat/completions")).toBe("/ui/v1/chat/completions");
      expect(at("/ui/", "/comfyui/")).toBe("/ui/comfyui/");
    });

    it("keeps the proxy prefix and adds the door behind it", () => {
      expect(at("/asubpath/llama-swap/ui/", "/api/profiles")).toBe(
        "/asubpath/llama-swap/ui/api/profiles",
      );
    });

    it("still uses the door when the document is not at the UI mount", () => {
      // The door is where auth.ui lives, so it is never skipped.
      expect(at("/", "/api/version")).toBe("/ui/api/version");
    });
  });
});
