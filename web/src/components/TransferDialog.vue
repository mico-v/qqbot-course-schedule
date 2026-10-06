<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import { Download, Upload } from "@element-plus/icons-vue";
import { downloadFile, uploadForm } from "../api";
import {
  canExport,
  errorMessage,
  importSummary,
  loadOverrides,
  loadScopes,
  reloadSelectedMember,
  safeFileLabel,
  selectedMemberLabel,
  state,
  timestamp,
} from "../store/admin";
import type { ImportResult, Scope } from "../types";

const visible = defineModel<boolean>("visible", { required: true });

const props = defineProps<{ scope: Scope | null }>();

const file = ref<File | null>(null);
const fileInput = ref<HTMLInputElement | null>(null);
const importing = ref(false);

const memberCount = computed(() => props.scope?.member_count || 0);
const memberHint = computed(() => selectedMemberLabel.value);

watch(visible, (open) => {
  if (!open) return;
  file.value = null;
  if (fileInput.value) fileInput.value.value = "";
});

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

async function exportArchive(format: "ics" | "backup"): Promise<void> {
  const scope = props.scope;
  if (!scope || !memberCount.value || !(await canExport())) return;
  const isBackup = format === "backup";
  const suffix = isBackup ? "原始备份" : "ICS";
  const extension = isBackup ? "json" : "zip";
  const filename = `课表-${suffix}-${safeFileLabel(scope.label)}-${timestamp()}.${extension}`;
  try {
    await downloadFile("/api/export", { scope_id: scope.scope_id, format }, filename);
    ElMessage.success(`已开始下载 ${filename}。`);
  } catch (error) {
    ElMessage.error(errorMessage(error));
  }
}

function onFileChange(event: Event): void {
  const input = event.target as HTMLInputElement;
  file.value = input.files?.[0] || null;
}

async function importArchive(): Promise<void> {
  const scope = props.scope;
  if (!scope || !file.value || importing.value) return;
  if (!(await canExport())) return;
  importing.value = true;
  try {
    const form = new FormData();
    form.append("scope_id", scope.scope_id);
    const name = file.value.name.toLocaleLowerCase();
    if (name.endsWith(".ics") && state.selectedScopeId === scope.scope_id && state.selectedUserId) {
      form.append("user_id", state.selectedUserId);
    }
    form.append("file", file.value);
    const result = await uploadForm<ImportResult>("/api/import", form);
    visible.value = false;
    await loadScopes();
    if (state.selectedScopeId && state.selectedUserId) {
      try {
        await reloadSelectedMember();
        await loadOverrides(state.selectedScopeId);
      } catch {
        /* member may have been replaced */
      }
    }
    ElMessage.success(importSummary(result));
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    importing.value = false;
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="导入 / 导出" width="600px" align-center destroy-on-close>
    <p class="dialog-hint muted">
      {{ scope?.label }} · {{ memberCount }} 位成员
    </p>
    <div class="transfer-section">
      <h4>导出</h4>
      <div class="transfer-actions">
        <el-button :icon="Download" :disabled="!memberCount" @click="exportArchive('ics')">
          导出 ICS 压缩包
        </el-button>
        <el-button
          :icon="Download"
          :disabled="!memberCount"
          @click="exportArchive('backup')"
        >
          导出原始备份
        </el-button>
      </div>
      <p class="muted transfer-note">ICS 压缩包适合迁移到其他日历；原始备份包含全部字段，可用于恢复。</p>
    </div>
    <el-divider />
    <div class="transfer-section">
      <h4>导入</h4>
      <div class="transfer-actions">
        <el-button :icon="Upload" @click="fileInput?.click()">选择文件</el-button>
        <span class="muted file-name">
          {{ file ? `${file.name} · ${formatSize(file.size)}` : "未选择文件" }}
        </span>
        <input
          ref="fileInput"
          class="file-input"
          type="file"
          accept=".zip,.ics,.json,application/zip,text/calendar,application/json"
          @change="onFileChange"
        />
      </div>
      <p class="muted transfer-note">
        {{
          memberHint
            ? `单个 .ics 会导入到当前选中的 ${memberHint}；文件名形如 schedule<OpenID>.ics 时以文件名为准。`
            : "本会话还没有选中成员：导入单个 .ics 前请先选中成员，或把文件命名为 schedule<OpenID>.ics。"
        }}
      </p>
    </div>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="importing" :disabled="!file" @click="importArchive">
        开始导入
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.dialog-hint {
  margin: -6px 0 12px;
  font-size: 13px;
}

.transfer-section h4 {
  margin: 0 0 10px;
  font-size: 14px;
}

.transfer-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.transfer-note {
  margin: 10px 0 0;
  font-size: 12px;
  line-height: 1.6;
}

.file-name {
  font-size: 12px;
}

.file-input {
  display: none;
}
</style>
