import { defineConfig } from 'vite';

export default defineConfig({
  // The local UI dependency is a symlink outside the dashboard's node_modules.
  resolve: { dedupe: ['react', 'react-dom'] },
});
