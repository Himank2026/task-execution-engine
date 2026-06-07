import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Vite config. The important bit is the dev proxy below.
export default defineConfig({
  plugins: [react()],
  server: {
    // The browser calls "/api/..." on the Vite dev server (port 5173), and Vite
    // forwards those calls to the Go backend (port 8080). Because the browser only
    // ever talks to one origin (5173), there is NO CORS to deal with. This also
    // streams Server-Sent Events through fine. In production, nginx plays this role.
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
