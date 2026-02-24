import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  // ОЧЕНЬ ВАЖНО: Указываем относительный базовый путь для корректной работы в Electron (file://)
  base: "./",
});
