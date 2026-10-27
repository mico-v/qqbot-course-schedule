import type { BotSettings } from "../types";

const DEFAULT_LINK_TTL_MINUTES = 60;

export function normalizeSettings(input: Partial<BotSettings>): BotSettings {
  return {
    enabled: input.enabled !== false,
    reply_plain: input.reply_plain !== false,
    reply_slash: input.reply_slash !== false,
    reply_mention: input.reply_mention !== false,
    send_format: input.send_format === "markdown" ? "markdown" : "image",
    nickname: input.nickname || "",
    base_url: input.base_url || "",
    schedule_link_ttl_minutes:
      Number.isFinite(input.schedule_link_ttl_minutes) &&
      Number(input.schedule_link_ttl_minutes) > 0
        ? Number(input.schedule_link_ttl_minutes)
        : DEFAULT_LINK_TTL_MINUTES,
    admin_open_ids: Array.isArray(input.admin_open_ids)
      ? input.admin_open_ids.filter((value) => typeof value === "string")
      : [],
  };
}
