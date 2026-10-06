import type { WebEvent } from "../types";
import {
  addDays,
  calendarDayDiff,
  parseLocalDateTime,
  sameCalendarDay,
  startOfDay,
  startOfWeek,
  weekdayCode,
} from "./datetime";

export interface Occurrence {
  event: WebEvent;
  eventIndex: number;
  start: Date;
  end: Date;
}

export interface DaySlice extends Occurrence {
  continuesBefore: boolean;
  continuesAfter: boolean;
}

export interface SliceLayout {
  column: number;
  columns: number;
}

export const CALENDAR_HOUR_HEIGHT = 54;
const CALENDAR_EVENT_COLORS = [
  "#4268df",
  "#0f8b8d",
  "#b45309",
  "#be123c",
  "#7c3aed",
  "#0369a1",
  "#4d7c0f",
  "#c2410c",
];

export function parseRRule(value: string | undefined | null): Record<string, string> {
  const fields: Record<string, string> = {};
  for (const part of String(value || "").split(";")) {
    const [key, fieldValue] = part.split("=", 2);
    if (key && fieldValue) fields[key.trim().toUpperCase()] = fieldValue.trim().toUpperCase();
  }
  return fields;
}

export function parseRRuleUntil(value: string | undefined): Date | null {
  const raw = String(value || "").trim();
  let match = /^(\d{4})(\d{2})(\d{2})$/.exec(raw);
  if (match) {
    return new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]), 23, 59, 59, 999);
  }
  match = /^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z$/.exec(raw);
  if (match) {
    return new Date(
      Date.UTC(
        Number(match[1]),
        Number(match[2]) - 1,
        Number(match[3]),
        Number(match[4]),
        Number(match[5]),
        Number(match[6]),
      ),
    );
  }
  match = /^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})$/.exec(raw);
  if (match) {
    return new Date(
      Number(match[1]),
      Number(match[2]) - 1,
      Number(match[3]),
      Number(match[4]),
      Number(match[5]),
      Number(match[6]),
    );
  }
  return null;
}

export function rruleMatchesDate(
  rule: Record<string, string>,
  eventStart: Date,
  candidate: Date,
): boolean {
  const frequency = rule.FREQ;
  if (!frequency) return sameCalendarDay(eventStart, candidate);
  const interval = Math.max(1, Number.parseInt(rule.INTERVAL || "1", 10) || 1);
  const dayDiff = calendarDayDiff(eventStart, candidate);
  if (dayDiff < 0) return false;

  if (frequency === "DAILY") return dayDiff % interval === 0;
  if (frequency === "WEEKLY") {
    const byDay = String(rule.BYDAY || weekdayCode(eventStart))
      .split(",")
      .map((value) => value.trim())
      .filter(Boolean);
    if (!byDay.includes(weekdayCode(candidate))) return false;
    const weekDiff = calendarDayDiff(startOfWeek(eventStart), startOfWeek(candidate)) / 7;
    return weekDiff >= 0 && weekDiff % interval === 0;
  }
  if (frequency === "MONTHLY") {
    const monthDiff =
      (candidate.getFullYear() - eventStart.getFullYear()) * 12 +
      candidate.getMonth() -
      eventStart.getMonth();
    if (monthDiff < 0 || monthDiff % interval !== 0) return false;
    const byMonthDay = String(rule.BYMONTHDAY || eventStart.getDate())
      .split(",")
      .map((value) => Number.parseInt(value, 10));
    return byMonthDay.includes(candidate.getDate());
  }
  if (frequency === "YEARLY") {
    const yearDiff = candidate.getFullYear() - eventStart.getFullYear();
    if (yearDiff < 0 || yearDiff % interval !== 0) return false;
    const month = Number.parseInt(rule.BYMONTH || String(eventStart.getMonth() + 1), 10);
    const monthDay = Number.parseInt(rule.BYMONTHDAY || String(eventStart.getDate()), 10);
    return candidate.getMonth() + 1 === month && candidate.getDate() === monthDay;
  }
  return false;
}

export function countRRuleOccurrences(
  rule: Record<string, string>,
  eventStart: Date,
  candidate: Date,
): number {
  let count = 0;
  for (
    let day = startOfDay(eventStart);
    calendarDayDiff(day, candidate) >= 0;
    day = addDays(day, 1)
  ) {
    const candidateAt = new Date(
      day.getFullYear(),
      day.getMonth(),
      day.getDate(),
      eventStart.getHours(),
      eventStart.getMinutes(),
    );
    if (rruleMatchesDate(rule, eventStart, candidateAt)) count += 1;
    if (count > 10000) break;
  }
  return count;
}

export function expandEventInWeek(
  event: WebEvent,
  eventIndex: number,
  weekStart: Date,
): Occurrence[] {
  const eventStart = parseLocalDateTime(event.start);
  const eventEnd = parseLocalDateTime(event.end);
  if (!eventStart || !eventEnd || eventEnd <= eventStart) return [];
  const duration = eventEnd.getTime() - eventStart.getTime();
  const weekEnd = addDays(weekStart, 7);
  const rule = parseRRule(event.rrule);
  const until = parseRRuleUntil(rule.UNTIL);
  const countLimit = Number.parseInt(rule.COUNT || "", 10);
  const occurrences: Occurrence[] = [];

  // Include the previous day so an overnight course remains visible in both days.
  for (let offset = -1; offset <= 6; offset += 1) {
    const day = addDays(weekStart, offset);
    const candidateStart = new Date(
      day.getFullYear(),
      day.getMonth(),
      day.getDate(),
      eventStart.getHours(),
      eventStart.getMinutes(),
    );
    if (!rruleMatchesDate(rule, eventStart, candidateStart)) continue;
    if (until && candidateStart > until) continue;
    if (countLimit && countRRuleOccurrences(rule, eventStart, candidateStart) > countLimit) {
      continue;
    }
    const candidateEnd = new Date(candidateStart.getTime() + duration);
    if (candidateEnd > weekStart && candidateStart < weekEnd) {
      occurrences.push({ event, eventIndex, start: candidateStart, end: candidateEnd });
    }
  }
  return occurrences;
}

export function weekOccurrences(events: WebEvent[], weekStart: Date): Occurrence[] {
  return events.flatMap((event, index) => expandEventInWeek(event, index, weekStart));
}

export function initialWeekStart(events: WebEvent[]): Date {
  const currentWeek = startOfWeek(new Date());
  if (weekOccurrences(events, currentWeek).length) return currentWeek;
  const starts = events
    .map((event) => parseLocalDateTime(event.start))
    .filter((value): value is Date => value !== null);
  starts.sort((left, right) => left.getTime() - right.getTime());
  return starts.length ? startOfWeek(starts[0]) : currentWeek;
}

export function calendarRange(occurrences: Occurrence[]): {
  startMinute: number;
  endMinute: number;
} {
  let startMinute = 8 * 60;
  let endMinute = 20 * 60;
  for (const occurrence of occurrences) {
    const lastDay = startOfDay(new Date(occurrence.end.getTime() - 1));
    for (
      let day = startOfDay(occurrence.start);
      calendarDayDiff(day, lastDay) >= 0;
      day = addDays(day, 1)
    ) {
      const dayStart = startOfDay(day);
      const dayEnd = addDays(dayStart, 1);
      const sliceStart = new Date(Math.max(occurrence.start.getTime(), dayStart.getTime()));
      const sliceEnd = new Date(Math.min(occurrence.end.getTime(), dayEnd.getTime()));
      if (sliceEnd <= sliceStart) continue;
      const start = Math.round((sliceStart.getTime() - dayStart.getTime()) / 60000);
      const end = Math.round((sliceEnd.getTime() - dayStart.getTime()) / 60000);
      startMinute = Math.min(startMinute, Math.floor(start / 60) * 60);
      endMinute = Math.max(endMinute, Math.ceil(Math.max(end, start + 30) / 60) * 60);
    }
  }
  if (endMinute - startMinute < 8 * 60) endMinute = Math.min(24 * 60, startMinute + 8 * 60);
  return { startMinute, endMinute };
}

export function eventColor(event: WebEvent): string {
  const text = `${event.course || ""}${event.uid || ""}`;
  let hash = 0;
  for (let index = 0; index < text.length; index += 1) {
    hash = (hash * 31 + text.charCodeAt(index)) | 0;
  }
  return CALENDAR_EVENT_COLORS[Math.abs(hash) % CALENDAR_EVENT_COLORS.length];
}

export function sliceOccurrencesForDay(occurrences: Occurrence[], day: Date): DaySlice[] {
  const dayStart = startOfDay(day);
  const dayEnd = addDays(dayStart, 1);
  return occurrences.flatMap((occurrence) => {
    const start = new Date(Math.max(occurrence.start.getTime(), dayStart.getTime()));
    const end = new Date(Math.min(occurrence.end.getTime(), dayEnd.getTime()));
    if (end <= start) return [];
    return [
      {
        ...occurrence,
        start,
        end,
        continuesBefore: occurrence.start < dayStart,
        continuesAfter: occurrence.end > dayEnd,
      },
    ];
  });
}

export function layoutDayOccurrences(slices: DaySlice[]): Map<DaySlice, SliceLayout> {
  const sorted = [...slices].sort(
    (left, right) => left.start.getTime() - right.start.getTime() || right.end.getTime() - left.end.getTime(),
  );
  const layout = new Map<DaySlice, SliceLayout>();
  let group: DaySlice[] = [];
  let groupEnd = 0;

  const flush = () => {
    if (!group.length) return;
    const columnEnds: number[] = [];
    const columns = new Map<DaySlice, number>();
    for (const occurrence of group) {
      let column = columnEnds.findIndex((end) => end <= occurrence.start.getTime());
      if (column < 0) {
        column = columnEnds.length;
        columnEnds.push(0);
      }
      columnEnds[column] = occurrence.end.getTime();
      columns.set(occurrence, column);
    }
    for (const occurrence of group) {
      layout.set(occurrence, { column: columns.get(occurrence) || 0, columns: columnEnds.length });
    }
    group = [];
    groupEnd = 0;
  };

  for (const occurrence of sorted) {
    if (group.length && occurrence.start.getTime() >= groupEnd) flush();
    group.push(occurrence);
    groupEnd = Math.max(groupEnd, occurrence.end.getTime());
  }
  flush();
  return layout;
}

export function simpleRepeatValue(value: string | undefined): string {
  const raw = String(value || "").trim();
  if (!raw) return "none";
  const rule = parseRRule(raw);
  if (!rule.FREQ) return "custom";
  const fields = Object.keys(rule);
  const hasSimpleInterval = !rule.INTERVAL || Number.parseInt(rule.INTERVAL, 10) === 1;
  const allowedFields =
    rule.FREQ === "WEEKLY"
      ? new Set(["FREQ", "INTERVAL", "BYDAY"])
      : new Set(["FREQ", "INTERVAL"]);
  if (!hasSimpleInterval || fields.some((field) => !allowedFields.has(field))) return "custom";
  if (rule.FREQ === "DAILY") return "daily";
  if (rule.FREQ === "WEEKLY") return "weekly";
  if (rule.FREQ === "MONTHLY") return "monthly";
  if (rule.FREQ === "YEARLY") return "yearly";
  return "custom";
}
