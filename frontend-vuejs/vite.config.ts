/// <reference types="vitest/config" />
import {defineConfig} from 'vite';
import vue from '@vitejs/plugin-vue';

export default defineConfig({
    plugins: [vue()],
    server: {
        fs: {
            // The theme CSS and fonts are shared with the vanilla GUI in
            // ../frontend (see src/main.ts).
            allow: ['..'],
        },
    },
    test: {
        environment: 'happy-dom',
    },
});
