<script setup lang="ts">
import AdminView from "./views/AdminView.vue";
import LoginView from "./views/LoginView.vue";
import PublicScheduleView from "./views/PublicScheduleView.vue";

// The Go server serves the same SPA bundle for /login and /admin; the path
// decides which view to mount.
const path = window.location.pathname.replace(/\/+$/, "");
const isLogin = path === "/login";
const isPublicSchedule = path === "/schedule/edit";
</script>

<template>
  <PublicScheduleView v-if="isPublicSchedule" />
  <LoginView v-else-if="isLogin" />
  <AdminView v-else />
</template>
