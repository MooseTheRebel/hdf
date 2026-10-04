/// <reference types="vitest/config" />
import {fileURLToPath} from 'node:url';
import {defineConfig} from 'vite';
import vue from '@vitejs/plugin-vue';

// The GUI text lives in the repository's top-level locales/ (shared with
// the vanilla GUI), imported as "@locales".
const locales = fileURLToPath(new URL('../../../locales', import.meta.url));

export default defineConfig({
    plugins: [vue()],
    resolve: {
        alias: {'@locales': locales},
    },
    server: {
        fs: {
            // The theme CSS and fonts are shared with the vanilla GUI (see
            // src/main.ts). Setting allow replaces Vite's default, so this
            // project's own root is listed too.
            allow: ['.', '../../hdf-gui-vanilla/frontend', locales],
        },
    },
    test: {
        environment: 'happy-dom',
    },
});
