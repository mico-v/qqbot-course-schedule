import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, requestJSON, withQuery } from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("requestJSON", () => {
  it("parses JSON and appends queries", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    await expect(requestJSON(withQuery("/api/test", { scope_id: "group:G1" }))).resolves.toEqual({
      ok: true,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/test?scope_id=group%3AG1", expect.any(Object));
  });

  it("maps stable server errors", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "冲突", code: "conflict" }), {
          status: 409,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    await expect(requestJSON("/api/test")).rejects.toMatchObject({
      message: "冲突",
      status: 409,
      code: "conflict",
    } satisfies Partial<ApiError>);
  });
});
