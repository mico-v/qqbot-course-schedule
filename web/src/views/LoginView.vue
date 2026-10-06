<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Lock, Right } from "@element-plus/icons-vue";
import { apiPost } from "../api";
import { errorMessage } from "../store/admin";

const password = ref("");
const loading = ref(false);
const error = ref("");

// Only allow same-site relative redirects from ?next=.
const next = computed(() => {
  const value = new URLSearchParams(window.location.search).get("next") || "";
  if (value.startsWith("/") && !value.startsWith("//")) return value;
  return "/admin";
});

onMounted(() => {
  document.title = "登录 · 课表管理";
});

async function submit(): Promise<void> {
  if (!password.value) {
    error.value = "请输入管理密码。";
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    await apiPost("/api/login", { password: password.value });
    window.location.replace(next.value);
  } catch (submitError) {
    error.value = errorMessage(submitError);
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="login-page">
    <div class="login-card">
      <div class="login-brand">
        <span class="login-logo">📅</span>
        <h1>课表管理</h1>
        <p>QQ 课表机器人管理台</p>
      </div>
      <form class="login-form" @submit.prevent="submit">
        <el-input
          v-model="password"
          type="password"
          size="large"
          placeholder="管理密码"
          :prefix-icon="Lock"
          show-password
          autofocus
          @keyup.enter="submit"
        />
        <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
        <el-button
          type="primary"
          size="large"
          class="login-submit"
          :loading="loading"
          :icon="Right"
          native-type="submit"
        >
          登录
        </el-button>
      </form>
      <p class="login-footnote">密码在服务器 config.json 的 admin_password 中配置</p>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 24px;
  background:
    radial-gradient(1000px 500px at 10% -10%, rgba(64, 108, 220, 0.18), transparent 60%),
    radial-gradient(800px 500px at 110% 110%, rgba(15, 139, 141, 0.16), transparent 60%),
    var(--kb-bg);
}

.login-card {
  width: min(400px, 100%);
  padding: 40px 36px 28px;
  border-radius: 18px;
  background: var(--kb-panel);
  border: 1px solid var(--kb-border);
  box-shadow: 0 18px 50px rgba(15, 23, 42, 0.12);
}

.login-brand {
  text-align: center;
  margin-bottom: 28px;
}

.login-logo {
  display: inline-grid;
  place-items: center;
  width: 64px;
  height: 64px;
  font-size: 32px;
  border-radius: 18px;
  background: linear-gradient(135deg, #4268df, #6f8cf0);
  box-shadow: 0 10px 24px rgba(66, 104, 223, 0.35);
}

.login-brand h1 {
  margin: 16px 0 6px;
  font-size: 24px;
  color: var(--kb-text);
}

.login-brand p {
  margin: 0;
  color: var(--kb-text-secondary);
  font-size: 14px;
}

.login-form {
  display: grid;
  gap: 16px;
}

.login-submit {
  width: 100%;
}

.login-footnote {
  margin: 24px 0 0;
  text-align: center;
  font-size: 12px;
  color: var(--kb-text-secondary);
}
</style>
