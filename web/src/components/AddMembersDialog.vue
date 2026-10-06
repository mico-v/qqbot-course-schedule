<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import { Refresh, Search } from "@element-plus/icons-vue";
import { createMemberSchedules, errorMessage, loadPendingMembers } from "../store/admin";
import type { PendingMember, Scope } from "../types";

const visible = defineModel<boolean>("visible", { required: true });

const props = defineProps<{ scope: Scope | null }>();

const members = ref<PendingMember[]>([]);
const selected = ref<Set<string>>(new Set());
const filter = ref("");
const loading = ref(false);
const submitting = ref(false);
const note = ref("");

const visibleMembers = computed(() => {
  const query = filter.value.trim().toLocaleLowerCase();
  if (!query) return members.value;
  return members.value.filter((member) =>
    `${member.name} ${member.user_id}`.toLocaleLowerCase().includes(query),
  );
});

const allVisibleSelected = computed(
  () => visibleMembers.value.length > 0 && visibleMembers.value.every((m) => selected.value.has(m.user_id)),
);

watch(visible, (open) => {
  if (!open) return;
  members.value = [];
  selected.value = new Set();
  filter.value = "";
  note.value = "";
  if (props.scope) void reload();
});

async function reload(): Promise<void> {
  if (!props.scope) return;
  loading.value = true;
  try {
    const data = await loadPendingMembers(props.scope.scope_id);
    members.value = data.members;
    note.value = data.note;
    selected.value = new Set();
  } catch (error) {
    members.value = [];
    note.value = errorMessage(error);
  } finally {
    loading.value = false;
  }
}

function toggle(userId: string): void {
  const next = new Set(selected.value);
  if (next.has(userId)) next.delete(userId);
  else next.add(userId);
  selected.value = next;
}

function toggleAllVisible(): void {
  const next = new Set(selected.value);
  if (allVisibleSelected.value) {
    for (const member of visibleMembers.value) next.delete(member.user_id);
  } else {
    for (const member of visibleMembers.value) next.add(member.user_id);
  }
  selected.value = next;
}

async function submit(): Promise<void> {
  if (!props.scope || selected.value.size === 0) {
    ElMessage.error("请先选择要添加课表的成员。");
    return;
  }
  submitting.value = true;
  try {
    const picked = members.value.filter((member) => selected.value.has(member.user_id));
    const created = await createMemberSchedules(props.scope.scope_id, picked);
    visible.value = false;
    ElMessage.success(`已创建 ${created} 位成员的空白课表。`);
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="添加成员课表" width="640px" align-center destroy-on-close>
    <p class="dialog-hint muted">{{ scope?.label }}</p>
    <div class="picker-toolbar">
      <el-input v-model="filter" :prefix-icon="Search" placeholder="搜索昵称或 OpenID" clearable />
      <el-button :icon="Refresh" :loading="loading" @click="reload">重新获取</el-button>
      <el-button :disabled="!visibleMembers.length" @click="toggleAllVisible">
        {{ allVisibleSelected ? "取消全选" : "全选可见" }}
      </el-button>
    </div>
    <div class="picker-list">
      <p v-if="loading" class="list-empty">正在读取成员…</p>
      <p v-else-if="!visibleMembers.length" class="list-empty">
        {{
          members.length
            ? "没有符合搜索条件的成员。"
            : note || "暂无可添加的成员。"
        }}
      </p>
      <label
        v-for="member in visibleMembers"
        :key="member.user_id"
        class="picker-row"
        :class="{ checked: selected.has(member.user_id) }"
      >
        <el-checkbox
          :model-value="selected.has(member.user_id)"
          @change="toggle(member.user_id)"
        />
        <span class="picker-text">
          <span class="picker-name">{{ member.name || "未命名成员" }}</span>
          <span class="picker-meta">{{ member.user_id }}</span>
        </span>
      </label>
    </div>
    <template #footer>
      <div class="dialog-footer">
        <span class="muted">已选 {{ selected.size }} 人</span>
        <span class="footer-spacer" />
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" :disabled="!selected.size" @click="submit">
          添加课表
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.dialog-hint {
  margin: -6px 0 12px;
  font-size: 13px;
}

.picker-toolbar {
  display: flex;
  gap: 10px;
  margin-bottom: 12px;
}

.picker-list {
  max-height: 380px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.picker-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border: 1px solid var(--kb-border);
  border-radius: 10px;
  cursor: pointer;
}

.picker-row.checked {
  border-color: var(--kb-accent);
  background: color-mix(in srgb, var(--kb-accent) 8%, transparent);
}

.picker-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.picker-name {
  font-size: 13px;
}

.picker-meta {
  font-size: 11px;
  color: var(--kb-text-secondary);
}

.dialog-footer {
  display: flex;
  align-items: center;
  width: 100%;
}

.footer-spacer {
  flex: 1;
}
</style>
