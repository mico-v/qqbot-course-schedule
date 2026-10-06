<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { setDirty, state } from "../store/admin";
import type { WebEvent } from "../types";
import {
  formatDateValue,
  formatDateTimeValue,
  formatTimeValue,
  parseLocalDateTime,
  sameCalendarDay,
  startOfWeek,
  timeValueFromMinutes,
  WEEKDAY_CODES,
  weekdayCode,
} from "../utils/datetime";
import { parseRRule, simpleRepeatValue } from "../utils/rrule";

const visible = defineModel<boolean>("visible", { required: true });

const props = defineProps<{
  index: number;
  defaults: { date: string; startTime: string; endTime: string } | null;
}>();

const emit = defineEmits<{
  (e: "saved", message: string): void;
}>();

const form = reactive({
  course: "",
  date: "",
  startTime: "08:00",
  endTime: "09:00",
  location: "",
  description: "",
  repeat: "none",
  weekdays: [] as string[],
  customRule: "",
});

const editingIndex = ref(-1);
const saving = ref(false);

const repeatOptions = [
  { value: "none", label: "不重复" },
  { value: "daily", label: "每天" },
  { value: "weekly", label: "每周" },
  { value: "monthly", label: "每月" },
  { value: "yearly", label: "每年" },
  { value: "custom", label: "自定义 RRULE" },
];

function initialize(): void {
  const events = state.schedule?.events || [];
  const event =
    props.index >= 0 && props.index < events.length ? (events[props.index] as WebEvent) : null;
  const now = new Date();
  const fallbackDate =
    state.calendarWeekStart && !sameCalendarDay(state.calendarWeekStart, startOfWeek(now))
      ? state.calendarWeekStart
      : now;
  const start = event ? parseLocalDateTime(event.start) : null;
  const end = event ? parseLocalDateTime(event.end) : null;

  editingIndex.value = event ? props.index : -1;
  form.course = event?.course || "";
  form.date =
    props.defaults?.date || (start ? formatDateValue(start) : formatDateValue(fallbackDate));
  form.startTime = props.defaults?.startTime || (start ? formatTimeValue(start) : "08:00");
  form.endTime =
    props.defaults?.endTime || (end ? formatTimeValue(end) : timeValueFromMinutes(9 * 60));
  form.location = event?.location || "";
  form.description = event?.description || "";
  form.customRule = event?.rrule || "";
  form.repeat = event ? simpleRepeatValue(event.rrule) : "none";

  const rule = parseRRule(event?.rrule);
  const weekdays = String(rule.BYDAY || "")
    .split(",")
    .filter((value) => WEEKDAY_CODES.includes(value));
  if (weekdays.length === 0 && start && form.repeat === "weekly") {
    weekdays.push(weekdayCode(start));
  }
  form.weekdays = weekdays;
}

watch(visible, (open) => {
  if (open) initialize();
});

watch(
  () => form.repeat,
  (repeat) => {
    if (repeat === "weekly" && form.weekdays.length === 0 && form.date) {
      const date = parseLocalDateTime(`${form.date}T00:00`);
      if (date) form.weekdays = [weekdayCode(date)];
    }
  },
);

function courseRRule(): string {
  if (form.repeat === "none") return "";
  if (form.repeat === "daily") return "FREQ=DAILY";
  if (form.repeat === "monthly") return "FREQ=MONTHLY";
  if (form.repeat === "yearly") return "FREQ=YEARLY";
  if (form.repeat === "custom") return form.customRule.trim();
  let weekdays = [...form.weekdays];
  if (weekdays.length === 0) {
    const date = parseLocalDateTime(`${form.date}T00:00`);
    if (date) weekdays = [weekdayCode(date)];
  }
  return `FREQ=WEEKLY${weekdays.length ? `;BYDAY=${weekdays.join(",")}` : ""}`;
}

function formDateTime(date: string, time: string, addDay = false): string | null {
  const parsed = parseLocalDateTime(`${date}T${time}`);
  if (!parsed) return null;
  if (addDay) parsed.setDate(parsed.getDate() + 1);
  return formatDateTimeValue(parsed);
}

function save(): void {
  const events = state.schedule?.events || [];
  if (!form.course.trim() || !form.date || !form.startTime || !form.endTime) {
    ElMessage.error("请填写课程名称、日期和上下课时间。");
    return;
  }
  if (form.startTime === form.endTime) {
    ElMessage.error("结束时间必须晚于开始时间。");
    return;
  }
  const crossesMidnight = form.endTime < form.startTime;
  const start = formDateTime(form.date, form.startTime);
  const end = formDateTime(form.date, form.endTime, crossesMidnight);
  if (!start || !end) {
    ElMessage.error("无法解析课程时间，请重新选择。");
    return;
  }
  const rrule = courseRRule();
  if (form.repeat === "custom" && !rrule) {
    ElMessage.error("请填写自定义重复规则。");
    return;
  }
  const previous = editingIndex.value >= 0 ? events[editingIndex.value] : null;
  const nextEvent: WebEvent = {
    id: previous?.id || 0,
    uid: previous?.uid || "",
    course: form.course.trim(),
    start,
    end,
    location: form.location.trim(),
    rrule,
    description: form.description.trim(),
  };
  if (editingIndex.value >= 0) events[editingIndex.value] = nextEvent;
  else events.push(nextEvent);
  setDirty(true);
  visible.value = false;
  emit("saved", editingIndex.value >= 0 ? "课程已更新，保存课表后生效。" : "课程已添加，保存课表后生效。");
}

async function remove(): Promise<void> {
  const events = state.schedule?.events || [];
  if (editingIndex.value < 0 || editingIndex.value >= events.length) return;
  const event = events[editingIndex.value];
  try {
    await ElMessageBox.confirm(`删除课程“${event.course || "未命名课程"}”？`, "删除课程", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  if (events.length === 1) {
    ElMessage.error("至少需要保留一节课程。");
    return;
  }
  events.splice(editingIndex.value, 1);
  setDirty(true);
  visible.value = false;
  emit("saved", "课程已删除，保存课表后生效。");
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="editingIndex >= 0 ? '编辑课程' : '添加课程'"
    width="600px"
    align-center
    destroy-on-close
  >
    <p class="dialog-hint muted">
      {{
        editingIndex >= 0
          ? "修改后保存课程，原课程标识会保留。"
          : "设置首次上课日期、时间和重复规则。"
      }}
    </p>
    <el-form label-position="top" @submit.prevent>
      <el-form-item label="课程名称" required>
        <el-input v-model="form.course" maxlength="200" placeholder="例如：高等数学" />
      </el-form-item>
      <div class="form-grid">
        <el-form-item label="首次上课日期" required>
          <el-date-picker
            v-model="form.date"
            type="date"
            value-format="YYYY-MM-DD"
            placeholder="选择日期"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="上课时间" required>
          <div class="time-range">
            <el-time-picker
              v-model="form.startTime"
              format="HH:mm"
              value-format="HH:mm"
              placeholder="开始"
              style="width: 100%"
            />
            <span class="muted">至</span>
            <el-time-picker
              v-model="form.endTime"
              format="HH:mm"
              value-format="HH:mm"
              placeholder="结束"
              style="width: 100%"
            />
          </div>
        </el-form-item>
      </div>
      <div class="form-grid">
        <el-form-item label="上课地点">
          <el-input v-model="form.location" maxlength="200" placeholder="例如：A101" />
        </el-form-item>
        <el-form-item label="重复">
          <el-select v-model="form.repeat" style="width: 100%">
            <el-option
              v-for="option in repeatOptions"
              :key="option.value"
              :label="option.label"
              :value="option.value"
            />
          </el-select>
        </el-form-item>
      </div>
      <el-form-item v-if="form.repeat === 'weekly'" label="每周上课日">
        <el-checkbox-group v-model="form.weekdays">
          <el-checkbox-button v-for="(code, index) in WEEKDAY_CODES" :key="code" :value="code">
            {{ "周" + "一二三四五六日"[index] }}
          </el-checkbox-button>
        </el-checkbox-group>
      </el-form-item>
      <el-form-item v-if="form.repeat === 'custom'" label="自定义 RRULE" required>
        <el-input
          v-model="form.customRule"
          placeholder="例如：FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE"
        />
      </el-form-item>
      <el-form-item label="备注">
        <el-input
          v-model="form.description"
          type="textarea"
          :rows="3"
          maxlength="2000"
          placeholder="教师、班级或其他备注"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <div class="dialog-footer">
        <el-button v-if="editingIndex >= 0" type="danger" plain @click="remove">删除课程</el-button>
        <span class="footer-spacer" />
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存课程</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.dialog-hint {
  margin: -6px 0 14px;
  font-size: 13px;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
}

.time-range {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.dialog-footer {
  display: flex;
  align-items: center;
  width: 100%;
}

.footer-spacer {
  flex: 1;
}

@media (max-width: 560px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
