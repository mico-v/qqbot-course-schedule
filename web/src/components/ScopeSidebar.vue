<script setup lang="ts">
import { computed, reactive } from "vue";
import { ArrowDown, ArrowRight, Plus, Search, Sort } from "@element-plus/icons-vue";
import { scopeMatches, selectScope, selectMember, state } from "../store/admin";
import type { Scope } from "../types";

const emit = defineEmits<{
  (e: "add-members", scope: Scope): void;
  (e: "transfer", scope: Scope): void;
}>();

const collapsed = reactive<Record<string, boolean>>({});

const visibleScopes = computed(() =>
  state.scopes.filter((scope) => scopeMatches(scope, state.search.trim())),
);

function isActive(scope: Scope): boolean {
  return state.selectedScopeId === scope.scope_id;
}

function isExpanded(scope: Scope): boolean {
  if (scope.scope_id in collapsed) return !collapsed[scope.scope_id];
  return true;
}

function toggle(scope: Scope): void {
  collapsed[scope.scope_id] = !collapsed[scope.scope_id];
}

function scopeMeta(scope: Scope): string {
  if (scope.member_count > 0) {
    return `${scope.member_count} 位成员 · ${scope.event_count} 节课`;
  }
  if (scope.pending_count > 0) return `待添加 ${scope.pending_count} 位成员`;
  return "0 位成员 · 0 节课";
}
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-head">
      <el-input
        v-model="state.search"
        :prefix-icon="Search"
        placeholder="搜索会话或成员"
        clearable
      />
      <span class="scope-count" :title="`共 ${state.scopes.length} 个会话`">
        {{ state.search ? visibleScopes.length : state.scopes.length }}
      </span>
    </div>
    <div class="sidebar-list">
      <p v-if="!visibleScopes.length" class="list-empty">
        {{ state.search ? "没有符合搜索条件的会话。" : "暂无可管理的会话。" }}
      </p>
      <div
        v-for="scope in visibleScopes"
        :key="scope.scope_id"
        class="scope-item"
        :class="{ active: isActive(scope) }"
      >
        <div class="scope-heading">
          <button
            type="button"
            class="scope-toggle"
            :aria-label="isExpanded(scope) ? '折叠会话' : '展开会话'"
            @click="toggle(scope)"
          >
            <el-icon>
              <ArrowDown v-if="isExpanded(scope)" />
              <ArrowRight v-else />
            </el-icon>
          </button>
          <button
            type="button"
            class="scope-button"
            :aria-current="isActive(scope) ? 'true' : undefined"
            @click="selectScope(scope)"
          >
            <span class="scope-label">{{ scope.label }}</span>
            <span class="scope-meta">{{ scopeMeta(scope) }}</span>
          </button>
          <el-tooltip v-if="scope.kind === 'group'" content="添加成员课表" placement="top">
            <button type="button" class="scope-action" @click="emit('add-members', scope)">
              <el-icon><Plus /></el-icon>
            </button>
          </el-tooltip>
          <el-tooltip content="导入 / 导出" placement="top">
            <button type="button" class="scope-action" @click="emit('transfer', scope)">
              <el-icon><Sort /></el-icon>
            </button>
          </el-tooltip>
        </div>
        <div v-if="isExpanded(scope)" class="member-list">
          <button
            v-for="member in scope.members || []"
            :key="member.user_id"
            type="button"
            class="member-item"
            :class="{
              active: isActive(scope) && state.selectedUserId === member.user_id,
            }"
            @click="selectMember(scope.scope_id, member.user_id)"
          >
            <span class="member-name">{{ member.name || member.user_id }}</span>
            <span class="member-meta">{{ member.user_id }} · {{ member.event_count || 0 }} 节课</span>
          </button>
          <p v-if="!(scope.members || []).length" class="member-empty">还没有成员课表</p>
        </div>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  width: 330px;
  flex: none;
  border-right: 1px solid var(--kb-border);
  background: var(--kb-panel);
  min-height: 0;
}

.sidebar-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px;
  border-bottom: 1px solid var(--kb-border);
}

.scope-count {
  display: inline-grid;
  place-items: center;
  min-width: 28px;
  height: 24px;
  padding: 0 8px;
  border-radius: 999px;
  background: var(--kb-panel-soft);
  border: 1px solid var(--kb-border);
  font-size: 12px;
  color: var(--kb-text-secondary);
}

.sidebar-list {
  flex: 1;
  overflow-y: auto;
  padding: 10px;
}

.scope-item + .scope-item {
  margin-top: 6px;
}

.scope-item.active {
  background: color-mix(in srgb, var(--kb-accent) 7%, transparent);
  border-radius: 12px;
}

.scope-heading {
  display: flex;
  align-items: center;
  gap: 2px;
}

.scope-toggle,
.scope-action {
  display: inline-grid;
  place-items: center;
  width: 26px;
  height: 26px;
  flex: none;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--kb-text-secondary);
  cursor: pointer;
}

.scope-toggle:hover,
.scope-action:hover {
  background: var(--kb-panel-soft);
  color: var(--kb-text);
}

.scope-button {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 7px 8px;
  border: none;
  border-radius: 10px;
  background: transparent;
  text-align: left;
  cursor: pointer;
  color: inherit;
  font: inherit;
}

.scope-button:hover {
  background: var(--kb-panel-soft);
}

.scope-label {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.scope-meta {
  font-size: 12px;
  color: var(--kb-text-secondary);
}

.member-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px 0 8px 26px;
}

.member-item {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 7px 10px;
  border: none;
  border-radius: 9px;
  background: transparent;
  text-align: left;
  cursor: pointer;
  color: inherit;
  font: inherit;
}

.member-item:hover {
  background: var(--kb-panel-soft);
}

.member-item.active {
  background: color-mix(in srgb, var(--kb-accent) 14%, transparent);
}

.member-item.active .member-name {
  color: var(--kb-accent);
}

.member-name {
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.member-meta {
  font-size: 11px;
  color: var(--kb-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.member-empty {
  margin: 2px 0 0;
  padding: 4px 10px;
  font-size: 12px;
  color: var(--kb-text-secondary);
}
</style>
