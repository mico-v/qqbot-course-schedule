<script setup lang="ts">
import { computed, ref } from "vue";
import { ElMessage } from "element-plus";
import { Download, Plus, Upload } from "@element-plus/icons-vue";
import { uploadForm } from "../api";
import {
  canExport,
  currentScope,
  errorMessage,
  exportMemberICS,
  importSummary,
  loadOverrides,
  loadScopes,
  reloadSelectedMember,
  saveSchedule,
  setDirty,
  state,
} from "../store/admin";
import type { ImportResult } from "../types";
import CourseDialog from "./CourseDialog.vue";
import OverridePanel from "./OverridePanel.vue";
import CheckinPanel from "./CheckinPanel.vue";
import WeekCalendar from "./WeekCalendar.vue";

const saving = ref(false);
const courseDialogVisible = ref(false);
const courseIndex = ref(-1);
const courseDefaults = ref<{ date: string; startTime: string; endTime: string } | null>(null);
const importInput = ref<HTMLInputElement | null>(null);

const schedule = computed(() => state.schedule);
const scopeLabel = computed(() => currentScope.value?.label || schedule.value?.scope_id || "");
const avatarUrl = computed(() => {
  const qq = (schedule.value?.qq || "").trim();
  if (!/^[1-9][0-9]{4,10}$/.test(qq)) return "";
  return `https://q1.qlogo.cn/g?b=qq&nk=${qq}&s=100`;
});

function onIdentityInput(): void {
  setDirty(true);
}

function openAddCourse(defaults: { date: string; startTime: string; endTime: string } | null = null): void {
  courseIndex.value = -1;
  courseDefaults.value = defaults;
  courseDialogVisible.value = true;
}

function openEditCourse(index: number): void {
  courseIndex.value = index;
  courseDefaults.value = null;
  courseDialogVisible.value = true;
}

async function submitSave(): Promise<void> {
  saving.value = true;
  try {
    await saveSchedule();
  } finally {
    saving.value = false;
  }
}

async function exportICS(): Promise<void> {
  await exportMemberICS();
}

async function onImportFile(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0] || null;
  input.value = "";
  if (!file || !schedule.value) return;
  if (!file.name.toLocaleLowerCase().endsWith(".ics")) {
    ElMessage.error("单个成员只能导入 .ics 文件；.zip / .json 请在会话的导入/导出对话框中使用。");
    return;
  }
  if (!(await canExport())) return;
  try {
    const form = new FormData();
    form.append("scope_id", schedule.value.scope_id);
    form.append("user_id", schedule.value.user_id);
    form.append("file", file);
    const result = await uploadForm<ImportResult>("/api/import", form);
    await loadScopes();
    await reloadSelectedMember();
    await loadOverrides(schedule.value.scope_id);
    ElMessage.success(importSummary(result));
  } catch (error) {
    ElMessage.error(errorMessage(error));
  }
}

function onCalendarAdd(defaults: { date: string; startTime: string; endTime: string }): void {
  openAddCourse(defaults);
}
</script>

<template>
  <div v-if="schedule" class="member-editor">
    <section class="panel identity-panel">
      <div class="identity-head">
        <div class="identity-title">
          <p class="section-kicker muted">{{ scopeLabel }}</p>
          <h2>{{ schedule.name || schedule.user_id }}</h2>
          <p class="muted identity-meta">
            {{ schedule.events.length }} 节课 · revision {{ schedule.revision }} · OpenID
            {{ schedule.user_id }}
          </p>
        </div>
        <div class="identity-actions">
          <el-tag v-if="state.dirty" type="warning" effect="light">有未保存修改</el-tag>
          <el-button type="primary" :loading="saving" @click="submitSave">保存课表</el-button>
        </div>
      </div>
      <div class="identity-grid">
        <div class="identity-field">
          <span class="field-label">成员名称</span>
          <el-input
            v-model="schedule.name"
            maxlength="200"
            placeholder="成员名称"
            @input="onIdentityInput"
          />
        </div>
        <div class="identity-field">
          <span class="field-label">QQ 号（用于头像预览）</span>
          <el-input
            v-model="schedule.qq"
            maxlength="11"
            placeholder="例如 123456789"
            @input="onIdentityInput"
          />
        </div>
        <div v-if="avatarUrl" class="identity-avatar">
          <img :src="avatarUrl" alt="QQ 头像预览" />
        </div>
      </div>
      <p class="muted identity-tip">修改名称不会改变成员身份。</p>
    </section>

    <section class="panel toolbar-panel">
      <div class="toolbar-left">
        <el-button type="primary" :icon="Plus" @click="openAddCourse()">添加课程</el-button>
        <el-button :icon="Upload" @click="importInput?.click()">导入 .ics</el-button>
        <el-button :icon="Download" @click="exportICS">导出 .ics</el-button>
        <input
          ref="importInput"
          class="file-input"
          type="file"
          accept=".ics,text/calendar"
          @change="onImportFile"
        />
      </div>
    </section>

    <WeekCalendar
      :events="schedule.events"
      :week-start="state.calendarWeekStart"
      @update:week-start="state.calendarWeekStart = $event"
      @add="onCalendarAdd"
      @edit="openEditCourse"
    />

    <div class="bottom-grid">
      <OverridePanel />
      <CheckinPanel />
    </div>

    <CourseDialog
      v-model:visible="courseDialogVisible"
      :index="courseIndex"
      :defaults="courseDefaults"
      :events="schedule.events"
      :week-start="state.calendarWeekStart"
      @saved="ElMessage.success($event)"
    />
  </div>
</template>

<style scoped>
.member-editor {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.identity-panel {
  padding: 18px;
}

.identity-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.identity-title h2 {
  margin: 2px 0 4px;
  font-size: 20px;
}

.section-kicker {
  margin: 0;
  font-size: 12px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.identity-meta {
  margin: 0;
  font-size: 12px;
}

.identity-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.identity-grid {
  display: grid;
  grid-template-columns: minmax(200px, 1fr) minmax(200px, 1fr) auto;
  gap: 14px;
  align-items: end;
  margin-top: 16px;
}

.identity-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field-label {
  font-size: 12px;
  color: var(--kb-text-secondary);
}

.identity-avatar img {
  width: 52px;
  height: 52px;
  border-radius: 12px;
  border: 1px solid var(--kb-border);
  object-fit: cover;
  display: block;
}

.identity-tip {
  margin: 10px 0 0;
  font-size: 12px;
}

.toolbar-panel {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 18px;
}

.toolbar-left {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.file-input {
  display: none;
}

.bottom-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 16px;
}

@media (max-width: 720px) {
  .identity-grid {
    grid-template-columns: 1fr;
  }
}
</style>
