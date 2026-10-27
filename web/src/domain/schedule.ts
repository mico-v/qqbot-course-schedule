import type { ImportResult, PageSchedule, WebEvent } from "../types";

export interface ScheduleSavePayload {
  scope_id: string;
  user_id: string;
  revision: number;
  name: string;
  qq: string;
  events: WebEvent[];
}

export function normalizeScheduleEvents(events: PageSchedule["events"] = []): WebEvent[] {
  return events.map((event) => ({
    id: event.id || 0,
    uid: event.uid || "",
    course: String(event.course || "").trim(),
    start: String(event.start || "").trim(),
    end: String(event.end || "").trim(),
    location: String(event.location || "").trim(),
    rrule: String(event.rrule || "").trim(),
    description: String(event.description || "").trim(),
  }));
}

export function validateScheduleEvents(events: WebEvent[]): string {
  if (!events.length) {
    return "至少需要一节课程；如果清空课表，请保留一节或删除该成员。";
  }
  for (const [index, event] of events.entries()) {
    if (!event.course) return `第 ${index + 1} 节缺少课程名称。`;
    if (!event.start || !event.end) return `第 ${index + 1} 节缺少开始或结束时间。`;
    if (event.end <= event.start) return `第 ${index + 1} 节的结束时间必须晚于开始时间。`;
  }
  return "";
}

export function buildScheduleSavePayload(schedule: PageSchedule): ScheduleSavePayload {
  return {
    scope_id: schedule.scope_id,
    user_id: schedule.user_id,
    revision: schedule.revision,
    name: schedule.name.trim(),
    qq: (schedule.qq || "").trim(),
    events: normalizeScheduleEvents(schedule.events),
  };
}

export function importSummary(result: ImportResult | null): string {
  const summary = result || ({} as ImportResult);
  const parts = [
    `已导入 ${summary.member_count || 0} 位成员的课表（新增 ${summary.created_count || 0}、` +
      `覆盖 ${summary.updated_count || 0}），共 ${summary.event_count || 0} 节课程。`,
  ];
  if (summary.day_override_count) {
    parts.push(`同时恢复 ${summary.day_override_count} 条休假/调休标记。`);
  }
  const skipped = Array.isArray(summary.skipped) ? summary.skipped : [];
  if (skipped.length) {
    const shown = skipped.slice(0, 3).join("、");
    parts.push(`忽略 ${skipped.length} 个文件：${shown}${skipped.length > 3 ? "…" : ""}`);
  }
  return parts.join("");
}

export function safeFileLabel(value: string | undefined): string {
  const cleaned = String(value || "scope")
    .replace(/[\\/:*?"<>|\s]+/g, "_")
    .slice(0, 40);
  return cleaned || "scope";
}

export function timestampLabel(now = new Date()): string {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(
    now.getHours(),
  )}${pad(now.getMinutes())}`;
}
