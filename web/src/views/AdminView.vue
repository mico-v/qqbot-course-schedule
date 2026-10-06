<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Moon, Refresh, Setting, Sunny, SwitchButton } from "@element-plus/icons-vue";
import { apiPost } from "../api";
import { currentScope, refresh, state } from "../store/admin";
import { cycleTheme, themeMode } from "../theme";
import type { Scope } from "../types";
import AddMembersDialog from "../components/AddMembersDialog.vue";
import MemberEditor from "../components/MemberEditor.vue";
import ScopeSidebar from "../components/ScopeSidebar.vue";
import SettingsDialog from "../components/SettingsDialog.vue";
import TransferDialog from "../components/TransferDialog.vue";

const settingsOpen = ref(false);
const addMembersOpen = ref(false);
const transferOpen = ref(false);
const dialogScope = ref<Scope | null>(null);

const themeLabel = computed(() => {
  if (themeMode.value === "auto") return "主题：自动";
  return themeMode.value === "dark" ? "主题：深色" : "主题：浅色";
});

const themeIcon = computed(() => (themeMode.value === "dark" ? Moon : Sunny));

function openAddMembers(scope: Scope): void {
  dialogScope.value = scope;
  addMembersOpen.value = true;
}

function openTransfer(scope: Scope): void {
  dialogScope.value = scope;
  transferOpen.value = true;
}

async function logout(): Promise<void> {
  try {
    await apiPost("/api/logout", {});
  } catch {
    /* 会话可能已过期，仍然回到登录页。 */
  }
  window.location.href = "/login";
}

onMounted(() => {
  document.title = "课表管理";
  void refresh();
});
</script>

<template>
  <div class="admin-shell">
    <header class="admin-header">
      <div class="brand">
        <span class="brand-logo">📅</span>
        <div class="brand-text">
          <strong>课表管理</strong>
          <span class="muted">{{ currentScope?.label || "从左侧选择会话与成员" }}</span>
        </div>
      </div>
      <div class="header-actions">
        <el-button :icon="Refresh" :loading="state.loading" @click="refresh">刷新数据</el-button>
        <el-button :icon="Setting" @click="settingsOpen = true">机器人设置</el-button>
        <el-button :icon="themeIcon" @click="cycleTheme">{{ themeLabel }}</el-button>
        <el-button :icon="SwitchButton" @click="logout">退出登录</el-button>
      </div>
    </header>
    <div class="admin-body">
      <ScopeSidebar @add-members="openAddMembers" @transfer="openTransfer" />
      <main class="admin-content">
        <MemberEditor v-if="state.schedule" />
        <div v-else class="empty-wrap">
          <el-empty :description="currentScope ? '从左侧选择成员开始编辑课表' : '请选择左侧的会话'">
            <div v-if="currentScope" class="empty-actions">
              <el-button
                v-if="currentScope.kind === 'group'"
                type="primary"
                @click="openAddMembers(currentScope)"
              >
                添加成员课表
              </el-button>
              <el-button @click="openTransfer(currentScope)">导入 / 导出</el-button>
            </div>
          </el-empty>
        </div>
      </main>
    </div>
    <SettingsDialog v-model:visible="settingsOpen" />
    <AddMembersDialog v-model:visible="addMembersOpen" :scope="dialogScope" />
    <TransferDialog v-model:visible="transferOpen" :scope="dialogScope" />
  </div>
</template>

<style scoped>
.admin-shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
}

.admin-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 10px 18px;
  background: var(--kb-panel);
  border-bottom: 1px solid var(--kb-border);
  flex: none;
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.brand-logo {
  display: inline-grid;
  place-items: center;
  width: 40px;
  height: 40px;
  font-size: 22px;
  border-radius: 12px;
  background: linear-gradient(135deg, #4268df, #6f8cf0);
  flex: none;
}

.brand-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.brand-text strong {
  font-size: 16px;
}

.brand-text span {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.header-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.admin-body {
  flex: 1;
  display: flex;
  min-height: 0;
}

.admin-content {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding: 18px;
}

.empty-wrap {
  height: 100%;
  display: grid;
  place-items: center;
}

.empty-actions {
  display: flex;
  gap: 10px;
  justify-content: center;
}

@media (max-width: 900px) {
  .admin-header {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
