import { describe, expect, it } from "vitest";
import { normalizeSettings } from "./settings";

describe("normalizeSettings", () => {
  it("keeps missing booleans enabled", () => {
    const settings = normalizeSettings({});
    expect(settings.enabled).toBe(true);
    expect(settings.reply_plain).toBe(true);
    expect(settings.reply_slash).toBe(true);
    expect(settings.reply_mention).toBe(true);
  });

  it("normalizes formats, limits, and administrator IDs", () => {
    const settings = normalizeSettings({
      send_format: "markdown",
      schedule_link_ttl_minutes: 0,
      admin_open_ids: [" ADMIN-1 ", "ADMIN-2"],
    });
    expect(settings.send_format).toBe("markdown");
    expect(settings.schedule_link_ttl_minutes).toBe(60);
    expect(settings.admin_open_ids).toEqual([" ADMIN-1 ", "ADMIN-2"]);
  });
});
