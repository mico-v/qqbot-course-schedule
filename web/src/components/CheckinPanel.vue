<script setup lang="ts">
import { Delete, Refresh } from "@element-plus/icons-vue";
import { deleteCheckin, loadCheckins, state } from "../store/admin";
</script>

<template>
  <section class="panel checkin-panel">
    <div class="panel-header">
      <h3 class="panel-title">群签到</h3>
      <el-button
        size="small"
        :icon="Refresh"
        :disabled="!state.selectedScopeId"
        @click="state.selectedScopeId && loadCheckins(state.selectedScopeId)"
      >
        刷新记录
      </el-button>
    </div>
    <div class="panel-body">
      <div v-if="state.checkinTotals.length" class="checkin-totals">
        <el-tag v-for="row in state.checkinTotals" :key="row.user_id" type="success" effect="light">
          {{ row.name || row.user_id }} · {{ row.points }} 分（{{ row.days }} 天）
        </el-tag>
      </div>
      <p v-if="!state.checkinRecords.length" class="list-empty">本会话还没有签到记录。</p>
      <div v-for="row in state.checkinRecords" :key="`${row.user_id}-${row.day}`" class="list-row">
        <div class="list-row-main">
          <strong class="list-row-title">
            {{ row.day }} · {{ row.name || row.user_id }} · +{{ row.points }}
          </strong>
          <span class="list-row-meta">OpenID {{ row.user_id }}</span>
        </div>
        <el-button
          size="small"
          type="danger"
          plain
          :icon="Delete"
          circle
          aria-label="删除签到记录"
          @click="deleteCheckin(row)"
        />
      </div>
    </div>
  </section>
</template>

<style scoped>
.checkin-panel {
  min-height: 200px;
}

.checkin-totals {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 12px;
}
</style>
