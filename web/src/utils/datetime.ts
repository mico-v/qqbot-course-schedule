import dayjs from "dayjs";

export const WEEKDAY_CODES = ["MO", "TU", "WE", "TH", "FR", "SA", "SU"];
export const WEEKDAY_LABELS = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];

export function padNumber(value: number): string {
  return String(value).padStart(2, "0");
}

export function formatDateValue(date: Date): string {
  return dayjs(date).format("YYYY-MM-DD");
}

export function formatTimeValue(date: Date): string {
  return dayjs(date).format("HH:mm");
}

export function formatDateTimeValue(date: Date): string {
  return dayjs(date).format("YYYY-MM-DDTHH:mm");
}

export function parseLocalDateTime(value: string | undefined | null): Date | null {
  if (!value) return null;
  const parsed = dayjs(value);
  return parsed.isValid() ? parsed.toDate() : null;
}

export function startOfDay(date: Date): Date {
  return dayjs(date).startOf("day").toDate();
}

export function startOfWeek(date: Date): Date {
  const day = dayjs(date).startOf("day");
  const offset = (day.day() + 6) % 7;
  return day.subtract(offset, "day").toDate();
}

export function addDays(date: Date, days: number): Date {
  return dayjs(date).startOf("day").add(days, "day").toDate();
}

export function calendarDayDiff(left: Date, right: Date): number {
  return dayjs(startOfDay(right)).diff(dayjs(startOfDay(left)), "day");
}

export function sameCalendarDay(left: Date, right: Date): boolean {
  return calendarDayDiff(left, right) === 0;
}

export function weekdayCode(date: Date): string {
  return WEEKDAY_CODES[(date.getDay() + 6) % 7];
}

export function timeValueFromMinutes(totalMinutes: number): string {
  const normalized = Math.max(0, Math.min(23 * 60 + 59, Math.round(totalMinutes)));
  return `${padNumber(Math.floor(normalized / 60))}:${padNumber(normalized % 60)}`;
}
