<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import { errorMessage, loadSettings, saveSettings } from "../store/admin";
import type { BotSettings } from "../types";

const visible = defineModel<boolean>("visible", { required: true });

const form = reactive<BotSettings>({
  enabled: true,
  reply_plain: true,
  reply_slash: true,
  reply_mention: true,
  send_format: "image",
  nickname: "",
  base_url: "",
  schedule_link_ttl_minutes: 60,
});

const loading = ref(false);
const saving = ref(false);
const hint = ref("");

watch(visible, async (open) => {
  if (!open) return;
  hint.value = "";
  loading.value = true;
  try {
    Object.assign(form, await loadSettings());
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    loading.value = false;
  }
});

async function submit(): Promise<void> {
  saving.value = true;
  try {
    await saveSettings({
      enabled: form.enabled,
      reply_plain: form.reply_plain,
      reply_slash: form.reply_slash,
      reply_mention: form.reply_mention,
      send_format: form.send_format,
      nickname: form.nickname.trim(),
      base_url: form.base_url.trim(),
      schedule_link_ttl_minutes: form.schedule_link_ttl_minutes,
    });
    hint.value = "已保存，立即生效。";
    ElMessage.success("机器人设置已保存。");
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="机器人设置" width="560px" align-center destroy-on-close>
    <div v-loading="loading">
      <el-form label-position="top">
        <el-form-item label="机器人开关">
          <el-switch v-model="form.enabled" active-text="启用" inactive-text="停用" />
          <span class="setting-hint muted">停用后机器人忽略所有消息。</span>
        </el-form-item>
        <el-form-item label="响应方式">
          <div class="switch-row">
            <el-checkbox v-model="form.reply_plain">无前缀（今日课表）</el-checkbox>
            <el-checkbox v-model="form.reply_slash">斜杠命令（/今日课表）</el-checkbox>
            <el-checkbox v-model="form.reply_mention">@ 机器人</el-checkbox>
          </div>
        </el-form-item>
        <el-form-item label="发送格式">
          <el-radio-group v-model="form.send_format">
            <el-radio value="image">图片课表（默认）</el-radio>
            <el-radio value="markdown">Markdown 列表</el-radio>
          </el-radio-group>
          <span class="setting-hint muted">Markdown 会以文字列表发送当天的课程内容。</span>
        </el-form-item>
        <el-form-item label="机器人昵称">
          <el-input
            v-model="form.nickname"
            maxlength="24"
            show-word-limit
            placeholder="例如：课表小助手（留空则不显示）"
          />
          <span class="setting-hint muted">昵称会显示在渲染的课表图片上。</span>
        </el-form-item>
        <el-form-item label="回调地址">
          <el-input
            v-model="form.base_url"
            placeholder="例如：https://kb.example.com"
            clearable
          />
          <span class="setting-hint muted">
            用于生成课表修改链接；留空时使用 config.json 的 public_base_url。
          </span>
        </el-form-item>
        <el-form-item label="课表连接过期时间">
          <el-input-number
            v-model="form.schedule_link_ttl_minutes"
            :min="5"
            :max="43200"
            :step="5"
            controls-position="right"
          />
          <span class="setting-hint muted">单位：分钟，最短 5 分钟，最长 30 天。</span>
        </el-form-item>
      </el-form>
      <p v-if="hint" class="save-hint">{{ hint }}</p>
    </div>
    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
      <el-button type="primary" :loading="saving" @click="submit">保存设置</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.switch-row {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 18px;
}

.setting-hint {
  display: block;
  width: 100%;
  font-size: 12px;
  line-height: 1.6;
}

.save-hint {
  margin: 0;
  color: var(--el-color-success);
  font-size: 13px;
}
</style>
