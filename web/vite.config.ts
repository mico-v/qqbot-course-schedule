import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

// Admin SPA is served by the Go binary under /admin; hashed assets live in
// /admin/assets and are embedded from dist into the executable.
export default defineConfig({
  base: "/admin/",
  plugins: [vue()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          "element-plus": ["element-plus", "@element-plus/icons-vue"],
          vendor: ["vue", "dayjs"],
        },
      },
    },
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:18080",
    },
  },
});
