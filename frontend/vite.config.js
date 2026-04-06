import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'
import fs from 'fs'

const packageJson = JSON.parse(fs.readFileSync("./package.json", "utf-8"));

export default defineConfig({
  plugins: [react()],
  define: {
    __APP_VERSION__: JSON.stringify(packageJson.version || "3.0.0"),
  }
})
