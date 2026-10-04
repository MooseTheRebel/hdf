/// <reference types="vitest/config" />
import {defineConfig} from 'vite';
import vue from '@vitejs/plugin-vue';

export default defineConfig({
    plugins: [vue()],
    server: {
        fs: {
            // The theme CSS and fonts are shared with the vanilla GUI (see
            // src/main.ts). Setting allow replaces Vite's default, so this
            // project's own root is listed too.
            allow: ['.', '../../hdf-gui-vanilla/frontend'],
        },
    },
    test: {
        environment: 'happy-dom',
    },
});
