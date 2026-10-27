import { describe, expect, it } from "vitest";
import type { PageSchedule, WebEvent } from "../types";
import {
  buildScheduleSavePayload,
  importSummary,
  normalizeScheduleEvents,
  safeFileLabel,
  timestampLabel,
  validateScheduleEvents,
} from "./schedule";

const event = (overrides: Partial<WebEvent> = {}): WebEvent => ({
  id: 1,
  uid: "uid-1",
  course: " 高数 ",
  location: " 教一 ",
  description: "",
  start: "2026-09-01T08:00",
  end: "2026-09-01T09:30",
  rrule: "",
  ...overrides,
});

describe("schedule domain", () => {
  it("normalizes and validates events", () => {
    const events = normalizeScheduleEvents([event()]);
    expect(events[0].course).toBe("高数");
    expect(validateScheduleEvents(events)).toBe("");
  });

  it("rejects an inverted time range", () => {
    const events = normalizeScheduleEvents([event({ end: "2026-09-01T07:00" })]);
    expect(validateScheduleEvents(events)).toContain("结束时间必须晚于开始时间");
  });

  it("builds a revision-bearing save command", () => {
    const schedule: PageSchedule = {
      scope_id: "group:G1",
      user_id: "M1",
      name: " 小明 ",
      qq: " 123456 ",
      revision: 4,
      events: [event()],
    };
    expect(buildScheduleSavePayload(schedule)).toMatchObject({
      scope_id: "group:G1",
      user_id: "M1",
      revision: 4,
      name: "小明",
      qq: "123456",
    });
  });

  it("formats import summaries and filenames", () => {
    expect(
      importSummary({
        member_count: 2,
        created_count: 1,
        updated_count: 1,
        event_count: 8,
        skipped: ["bad.ics"],
      }),
    ).toContain("忽略 1 个文件");
    expect(safeFileLabel("群聊 G1/测试")).toBe("群聊_G1_测试");
    expect(timestampLabel(new Date(2026, 9, 27, 8, 5))).toBe("20261027-0805");
  });
});
