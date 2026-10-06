<script setup lang="ts">
import { computed } from "vue";
import { ArrowLeft, ArrowRight } from "@element-plus/icons-vue";
import type { WebEvent } from "../types";
import {
  addDays,
  formatDateValue,
  formatTimeValue,
  padNumber,
  sameCalendarDay,
  startOfWeek,
  timeValueFromMinutes,
  WEEKDAY_LABELS,
} from "../utils/datetime";
import {
  CALENDAR_HOUR_HEIGHT,
  calendarRange,
  eventColor,
  layoutDayOccurrences,
  sliceOccurrencesForDay,
  weekOccurrences,
} from "../utils/rrule";

const props = defineProps<{
  events: WebEvent[];
  weekStart: Date;
}>();

const emit = defineEmits<{
  (e: "update:weekStart", value: Date): void;
  (e: "add", defaults: { date: string; startTime: string; endTime: string }): void;
  (e: "edit", index: number): void;
}>();

const minuteHeight = CALENDAR_HOUR_HEIGHT / 60;

const normalizedWeekStart = computed(() => startOfWeek(props.weekStart));
const occurrences = computed(() => weekOccurrences(props.events || [], normalizedWeekStart.value));
const range = computed(() => calendarRange(occurrences.value));
const bodyHeight = computed(
  () => (range.value.endMinute - range.value.startMinute) * minuteHeight,
);
const days = computed(() =>
  Array.from({ length: 7 }, (_, index) => addDays(normalizedWeekStart.value, index)),
);
const hourLabels = computed(() => {
  const labels: Array<{ top: number; text: string }> = [];
  for (let minute = range.value.startMinute; minute <= range.value.endMinute; minute += 60) {
    labels.push({
      top: (minute - range.value.startMinute) * minuteHeight,
      text: `${padNumber(Math.floor(minute / 60) % 24)}:00`,
    });
  }
  return labels;
});

const dayData = computed(() =>
  days.value.map((day) => {
    const slices = sliceOccurrencesForDay(occurrences.value, day);
    const layout = layoutDayOccurrences(slices);
    const positioned = slices.map((slice) => {
      const position = layout.get(slice) || { column: 0, columns: 1 };
      const startMinute =
        Math.round((slice.start.getTime() - day.getTime()) / 60000) - range.value.startMinute;
      const endMinute =
        Math.round((slice.end.getTime() - day.getTime()) / 60000) - range.value.startMinute;
      const top = Math.max(0, startMinute * minuteHeight);
      const height = Math.max(34, (endMinute - startMinute) * minuteHeight);
      return {
        slice,
        top,
        height,
        left: `calc(${(position.column / position.columns) * 100}% + 3px)`,
        width: `calc(${100 / position.columns}% - 6px)`,
        accent: eventColor(slice.event),
        compact: height < 66,
        tiny: height < 46,
      };
    });
    return { day, positioned };
  }),
);

const weekRangeText = computed(() => {
  const start = normalizedWeekStart.value;
  const end = addDays(start, 6);
  if (start.getFullYear() === end.getFullYear()) {
    return `${start.getFullYear()}年${start.getMonth() + 1}月${start.getDate()}日 - ${
      end.getMonth() + 1
    }月${end.getDate()}日`;
  }
  return `${start.getFullYear()}年${start.getMonth() + 1}月${start.getDate()}日 - ${
    end.getFullYear()
  }年${end.getMonth() + 1}月${end.getDate()}日`;
});

function shiftWeek(offset: number): void {
  emit("update:weekStart", addDays(normalizedWeekStart.value, offset * 7));
}

function goToday(): void {
  emit("update:weekStart", startOfWeek(new Date()));
}

function onDayClick(day: Date, event: MouseEvent): void {
  const target = event.target as HTMLElement | null;
  if (target?.closest(".calendar-event")) return;
  const column = event.currentTarget as HTMLElement | null;
  let startMinute = 8 * 60;
  if (column) {
    const rect = column.getBoundingClientRect();
    const offset = Math.max(0, event.clientY - rect.top);
    startMinute = range.value.startMinute + offset / minuteHeight;
    startMinute = Math.round(startMinute / 30) * 30;
  }
  startMinute = Math.max(0, Math.min(23 * 60 + 30, startMinute));
  emit("add", {
    date: formatDateValue(day),
    startTime: timeValueFromMinutes(startMinute),
    endTime: timeValueFromMinutes(Math.min(23 * 60 + 59, startMinute + 60)),
  });
}

function onDayKeydown(day: Date, event: KeyboardEvent): void {
  if (event.key !== "Enter" && event.key !== " ") return;
  event.preventDefault();
  emit("add", { date: formatDateValue(day), startTime: "08:00", endTime: "09:00" });
}

function eventLabel(positioned: (typeof dayData.value)[number]["positioned"][number]): string {
  const { slice } = positioned;
  const location = slice.event.location ? `，地点 ${slice.event.location}` : "";
  return `${slice.event.course || "未命名课程"}，${formatTimeValue(slice.start)} 至 ${formatTimeValue(
    slice.end,
  )}${location}，点击编辑`;
}
</script>

<template>
  <div class="calendar-panel panel">
    <div class="panel-header calendar-toolbar">
      <div class="calendar-nav">
        <el-button-group>
          <el-button :icon="ArrowLeft" @click="shiftWeek(-1)">上一周</el-button>
          <el-button @click="goToday">本周</el-button>
          <el-button @click="shiftWeek(1)">
            下一周
            <el-icon class="el-icon--right"><ArrowRight /></el-icon>
          </el-button>
        </el-button-group>
      </div>
      <div class="calendar-info">
        <span class="calendar-range">{{ weekRangeText }}</span>
        <el-tag size="small" type="info">{{ occurrences.length }} 节</el-tag>
      </div>
    </div>
    <div class="calendar-scroll">
      <div class="week-calendar">
        <div class="calendar-corner">时间</div>
        <div
          v-for="(day, index) in days"
          :key="`head-${index}`"
          class="calendar-day-head"
          :class="{ today: sameCalendarDay(day, new Date()) }"
        >
          <strong>{{ WEEKDAY_LABELS[index] }}</strong>
          <span>{{ day.getMonth() + 1 }}/{{ day.getDate() }}</span>
        </div>
        <div class="calendar-time-axis" :style="{ height: `${bodyHeight}px` }">
          <span
            v-for="label in hourLabels"
            :key="`${label.text}-${label.top}`"
            class="calendar-time-label"
            :style="{ top: `${label.top}px` }"
          >
            {{ label.text }}
          </span>
        </div>
        <div
          v-for="(data, index) in dayData"
          :key="`day-${index}`"
          class="calendar-day"
          :class="{ today: sameCalendarDay(data.day, new Date()) }"
          :style="{ height: `${bodyHeight}px` }"
          tabindex="0"
          :aria-label="`${WEEKDAY_LABELS[index]}，${data.day.getMonth() + 1}月${data.day.getDate()}日，按回车添加课程`"
          @click="onDayClick(data.day, $event)"
          @keydown="onDayKeydown(data.day, $event)"
        >
          <button
            v-for="positioned in data.positioned"
            :key="`${positioned.slice.eventIndex}-${positioned.slice.start.getTime()}`"
            type="button"
            class="calendar-event"
            :class="{ compact: positioned.compact, tiny: positioned.tiny }"
            :style="{
              top: `${positioned.top}px`,
              height: `${positioned.height}px`,
              left: positioned.left,
              width: positioned.width,
              '--event-accent': positioned.accent,
            }"
            :title="`${positioned.slice.event.course || '未命名课程'} ${formatTimeValue(
              positioned.slice.start,
            )}-${formatTimeValue(positioned.slice.end)}`"
            :aria-label="eventLabel(positioned)"
            @click.stop="emit('edit', positioned.slice.eventIndex)"
          >
            <span class="calendar-event-time">
              {{ formatTimeValue(positioned.slice.start) }} -
              {{ formatTimeValue(positioned.slice.end) }}
            </span>
            <strong class="calendar-event-title">
              {{ positioned.slice.event.course || "未命名课程" }}
            </strong>
            <span class="calendar-event-location">{{ positioned.slice.event.location }}</span>
          </button>
        </div>
      </div>
    </div>
    <p v-if="!occurrences.length" class="calendar-empty muted">
      本周暂无课程，点击日历空白处或「添加课程」创建第一节课。
    </p>
  </div>
</template>

<style scoped>
.calendar-panel {
  overflow: hidden;
}

.calendar-toolbar {
  flex-wrap: wrap;
}

.calendar-info {
  display: flex;
  align-items: center;
  gap: 10px;
}

.calendar-range {
  font-size: 13px;
  color: var(--kb-text-secondary);
}

.calendar-scroll {
  overflow-x: auto;
}

.week-calendar {
  display: grid;
  grid-template-columns: 64px repeat(7, minmax(120px, 1fr));
  min-width: 900px;
}

.calendar-corner {
  display: grid;
  place-items: center;
  padding: 8px 0;
  font-size: 12px;
  color: var(--kb-text-secondary);
  border-bottom: 1px solid var(--kb-border);
}

.calendar-day-head {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1px;
  padding: 8px 0;
  border-bottom: 1px solid var(--kb-border);
  border-left: 1px solid var(--kb-border);
  font-size: 12px;
  color: var(--kb-text-secondary);
}

.calendar-day-head strong {
  font-size: 13px;
  color: var(--kb-text);
}

.calendar-day-head.today strong,
.calendar-day-head.today span {
  color: var(--kb-accent);
}

.calendar-time-axis {
  position: relative;
}

.calendar-time-label {
  position: absolute;
  right: 8px;
  transform: translateY(-50%);
  font-size: 11px;
  color: var(--kb-text-secondary);
}

.calendar-day {
  position: relative;
  border-left: 1px solid var(--kb-border);
  background:
    linear-gradient(to bottom, transparent calc(100% - 1px), var(--kb-border) calc(100% - 1px))
      0 0 / 100% 54px;
  cursor: pointer;
  outline: none;
}

.calendar-day:focus-visible {
  box-shadow: inset 0 0 0 2px var(--kb-accent);
}

.calendar-day.today {
  background-color: color-mix(in srgb, var(--kb-accent) 5%, transparent);
}

.calendar-event {
  position: absolute;
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 5px 7px;
  border: none;
  border-left: 3px solid var(--event-accent);
  border-radius: 7px;
  background: color-mix(in srgb, var(--event-accent) 14%, var(--kb-panel));
  color: var(--kb-text);
  text-align: left;
  cursor: pointer;
  overflow: hidden;
  font: inherit;
  z-index: 1;
}

.calendar-event:hover {
  filter: brightness(1.05);
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.15);
}

.calendar-event-time {
  font-size: 10px;
  color: var(--kb-text-secondary);
  white-space: nowrap;
}

.calendar-event-title {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.calendar-event-location {
  font-size: 10px;
  color: var(--kb-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.calendar-event.compact .calendar-event-location {
  display: none;
}

.calendar-event.tiny .calendar-event-time {
  display: none;
}

.calendar-empty {
  margin: 0;
  padding: 12px 18px 16px;
  font-size: 13px;
}
</style>
