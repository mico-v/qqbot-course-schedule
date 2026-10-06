<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { Check, Plus } from "@element-plus/icons-vue";
import { ApiError, publicApiGet, publicApiPost } from "../api";
import CourseDialog from "../components/CourseDialog.vue";
import WeekCalendar from "../components/WeekCalendar.vue";
import type { PublicPageSchedule, PublicSaveResult, WebEvent } from "../types";
import { initialWeekStart } from "../utils/rrule";

const token = new URLSearchParams(window.location.search).get("token") || "";
const schedule = ref<PublicPageSchedule | null>(null);
const events = ref<WebEvent[]>([]);
const weekStart = ref(initialWeekStart([]));
const loading = ref(true);
const saving = ref(false);
const dirty = ref(false);
const loadError = ref("");
const expired = ref(false);
const dialogVisible = ref(false);
const courseIndex = ref(-1);
const courseDefaults = ref<{ date: string; startTime: string; endTime: string } | null>(null);

const expiresText = computed(() => {
  const raw = schedule.value?.expires_at || "";
  if (!raw) return "";
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
});

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

async function load(): Promise<void> {
  if (!token) {
    loading.value = false;
    loadError.value = "修改链接缺少访问标识。";
    return;
  }
  try {
    const page = await publicApiGet<PublicPageSchedule>("/api/public/schedule", { token });
    schedule.value = page;
    events.value = page.events || [];
    weekStart.value = initialWeekStart(events.value);
  } catch (error) {
    expired.value = error instanceof ApiError && error.status === 410;
    loadError.value = errorText(error);
  } finally {
    loading.value = false;
  }
}

function openAddCourse(
  defaults: { date: string; startTime: string; endTime: string } | null = null,
): void {
  courseIndex.value = -1;
  courseDefaults.value = defaults;
  dialogVisible.value = true;
}

function openEditCourse(index: number): void {
  courseIndex.value = index;
  courseDefaults.value = null;
  dialogVisible.value = true;
}

function validateEvents(): string {
  if (!events.value.length) {
    return "至少需要保留一节课程。";
  }
  for (const [index, event] of events.value.entries()) {
    if (!String(event.course || "").trim()) {
      return `第 ${index + 1} 节缺少课程名称。`;
    }
    if (!event.start || !event.end) {
      return `第 ${index + 1} 节缺少开始或结束时间。`;
    }
    if (event.end <= event.start) {
      return `第 ${index + 1} 节的结束时间必须晚于开始时间。`;
    }
  }
  return "";
}

async function save(): Promise<void> {
  const page = schedule.value;
  if (!page) return;
  const problem = validateEvents();
  if (problem) {
    ElMessage.error(problem);
    return;
  }
  saving.value = true;
  try {
    const result = await publicApiPost<PublicSaveResult>(
      `/api/public/schedule/save?token=${encodeURIComponent(token)}`,
      {
        revision: page.revision,
        name: page.name.trim(),
        qq: (page.qq || "").trim(),
        events: events.value.map((event) => ({
          id: event.id || 0,
          uid: event.uid || "",
          course: String(event.course || "").trim(),
          start: String(event.start || "").trim(),
          end: String(event.end || "").trim(),
          location: String(event.location || "").trim(),
          rrule: String(event.rrule || "").trim(),
          description: String(event.description || "").trim(),
        })),
      },
    );
    page.revision = result.revision;
    page.name = result.name;
    dirty.value = false;
    ElMessage.success(`已保存 ${result.event_count} 节课。`);
  } catch (error) {
    if (error instanceof ApiError && error.status === 409) {
      ElMessage.error("课表已被其他操作更新，请刷新后重试。");
      return;
    }
    if (error instanceof ApiError && error.status === 410) {
      expired.value = true;
    }
    ElMessage.error(errorText(error));
  } finally {
    saving.value = false;
  }
}

function markDirty(): void {
  dirty.value = true;
}

function onBeforeUnload(event: BeforeUnloadEvent): void {
  if (!dirty.value) return;
  event.preventDefault();
}

onMounted(() => {
  document.title = "课表修改";
  window.addEventListener("beforeunload", onBeforeUnload);
  void load();
});

onBeforeUnmount(() => {
  window.removeEventListener("beforeunload", onBeforeUnload);
});
</script>

<template>
  <div class="public-shell">
    <header class="public-header">
      <div>
        <p class="public-kicker muted">课程表</p>
        <h1>修改课表</h1>
      </div>
      <div v-if="schedule" class="public-meta">
        <strong>{{ schedule.name }}</strong>
        <span class="muted">链接有效至 {{ expiresText }}</span>
      </div>
    </header>

    <main class="public-content">
      <div v-if="loading" class="public-state panel" v-loading="true" />

      <div v-else-if="loadError" class="public-state panel">
        <el-result
          :icon="expired ? 'warning' : 'error'"
          :title="expired ? '链接已过期' : '无法打开课表'"
          :sub-title="loadError"
        >
          <template #extra>
            <p class="muted">请返回聊天重新发送 /修改课程表 获取新链接。</p>
          </template>
        </el-result>
      </div>

      <template v-else-if="schedule">
        <section class="panel public-identity">
          <div class="public-identity-fields">
            <label>
              <span>成员名称</span>
              <el-input
                v-model="schedule.name"
                maxlength="200"
                placeholder="成员名称"
                @input="markDirty"
              />
            </label>
            <label>
              <span>QQ 号</span>
              <el-input
                v-model="schedule.qq"
                maxlength="11"
                placeholder="可选"
                @input="markDirty"
              />
            </label>
          </div>
          <el-button type="primary" :icon="Check" :loading="saving" @click="save">
            保存课表
          </el-button>
        </section>

        <section class="panel public-toolbar">
          <el-button type="primary" :icon="Plus" @click="openAddCourse()">添加课程</el-button>
          <span v-if="dirty" class="dirty-text">有未保存修改</span>
        </section>

        <WeekCalendar
          :events="events"
          :week-start="weekStart"
          @update:week-start="weekStart = $event"
          @add="openAddCourse"
          @edit="openEditCourse"
        />
      </template>
    </main>

    <CourseDialog
      v-model:visible="dialogVisible"
      :index="courseIndex"
      :defaults="courseDefaults"
      :events="events"
      :week-start="weekStart"
      :mark-dirty="false"
      @saved="markDirty"
    />
  </div>
</template>

<style scoped>
.public-shell {
  min-height: 100%;
  background: var(--kb-bg);
}

.public-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  max-width: 1440px;
  margin: 0 auto;
  padding: 22px 28px 14px;
}

.public-kicker {
  margin: 0 0 2px;
  font-size: 12px;
}

.public-header h1 {
  margin: 0;
  font-size: 24px;
}

.public-meta {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 3px;
  text-align: right;
  font-size: 13px;
}

.public-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 1440px;
  margin: 0 auto;
  padding: 8px 28px 32px;
}

.public-state {
  min-height: 320px;
  display: grid;
  place-items: center;
}

.public-state :deep(.el-result) {
  width: min(560px, 100%);
}

.public-identity,
.public-toolbar {
  padding: 16px 18px;
}

.public-identity {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 18px;
}

.public-identity-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(180px, 1fr));
  gap: 14px;
  flex: 1;
}

.public-identity-fields label {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.public-identity-fields span {
  color: var(--kb-text-secondary);
  font-size: 12px;
}

.public-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.dirty-text {
  color: #b45309;
  font-size: 13px;
}

@media (max-width: 760px) {
  .public-header {
    align-items: flex-start;
    flex-direction: column;
    padding: 18px 16px 10px;
  }

  .public-meta {
    align-items: flex-start;
    text-align: left;
  }

  .public-content {
    padding: 6px 16px 24px;
  }

  .public-identity {
    align-items: stretch;
    flex-direction: column;
  }

  .public-identity-fields {
    grid-template-columns: 1fr;
  }
}
</style>
