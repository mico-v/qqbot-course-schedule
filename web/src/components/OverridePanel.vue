<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { ElMessage } from "element-plus";
import { Delete, Plus } from "@element-plus/icons-vue";
import { currentScope, deleteOverride, errorMessage, setOverride, state } from "../store/admin";
import { formatDateValue } from "../utils/datetime";

const formOpen = ref(false);
const submitting = ref(false);

const form = reactive({
  day: formatDateValue(new Date()),
  kind: "holiday",
  sourceDay: "",
  userId: "*",
});

const memberOptions = computed(() => {
  const scope = currentScope.value;
  return [
    { value: "*", label: "全体成员" },
    ...(scope?.members || []).map((member) => ({
      value: member.user_id,
      label: member.name || member.user_id,
    })),
  ];
});

function kindText(kind: string): string {
  return kind === "shift" ? "调休" : "休假";
}

function openForm(): void {
  form.day = form.day || formatDateValue(new Date());
  form.kind = "holiday";
  form.sourceDay = "";
  form.userId = state.selectedUserId || "*";
  formOpen.value = true;
}

async function submit(): Promise<void> {
  if (!form.day) {
    ElMessage.error("请选择标记日期。");
    return;
  }
  if (form.kind === "shift" && !form.sourceDay) {
    ElMessage.error("调休需要选择来源日期。");
    return;
  }
  submitting.value = true;
  try {
    await setOverride({
      user_id: form.userId,
      day: form.day,
      kind: form.kind,
      source_day: form.sourceDay,
    });
    formOpen.value = false;
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <section class="panel override-panel">
    <div class="panel-header">
      <h3 class="panel-title">休假 / 调休</h3>
      <el-button size="small" type="primary" plain :icon="Plus" @click="openForm">添加标记</el-button>
    </div>
    <div class="panel-body">
      <div v-if="formOpen" class="override-form">
        <div class="override-form-grid">
          <el-date-picker
            v-model="form.day"
            type="date"
            value-format="YYYY-MM-DD"
            placeholder="标记日期"
            style="width: 100%"
          />
          <el-select v-model="form.kind" style="width: 100%">
            <el-option label="休假（当天课程全部取消）" value="holiday" />
            <el-option label="调休（按来源日期上课）" value="shift" />
          </el-select>
          <el-date-picker
            v-if="form.kind === 'shift'"
            v-model="form.sourceDay"
            type="date"
            value-format="YYYY-MM-DD"
            placeholder="来源日期"
            style="width: 100%"
          />
          <el-select v-model="form.userId" style="width: 100%">
            <el-option
              v-for="option in memberOptions"
              :key="option.value"
              :label="option.label"
              :value="option.value"
            />
          </el-select>
        </div>
        <div class="override-form-actions">
          <el-button size="small" @click="formOpen = false">取消</el-button>
          <el-button size="small" type="primary" :loading="submitting" @click="submit">
            保存标记
          </el-button>
        </div>
      </div>
      <p v-if="!state.overrides.length && !formOpen" class="list-empty">暂无休假/调休标记。</p>
      <div v-for="row in state.overrides" :key="`${row.user_id}-${row.day}`" class="list-row">
        <div class="list-row-main">
          <strong class="list-row-title">{{ row.day }} · {{ kindText(row.kind) }}</strong>
          <span class="list-row-meta">
            {{
              row.kind === "shift" && row.source_day
                ? `${row.name} · 按 ${row.source_day} 的课程上课`
                : `${row.name} · 当天课程全部取消`
            }}
          </span>
        </div>
        <el-button
          size="small"
          type="danger"
          plain
          :icon="Delete"
          circle
          aria-label="删除标记"
          @click="deleteOverride(row)"
        />
      </div>
    </div>
  </section>
</template>

<style scoped>
.override-panel {
  min-height: 200px;
}

.override-form {
  margin-bottom: 12px;
  padding: 12px;
  border: 1px dashed var(--kb-border);
  border-radius: 10px;
  background: var(--kb-panel-soft);
}

.override-form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 10px;
}

.override-form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 10px;
}
</style>
