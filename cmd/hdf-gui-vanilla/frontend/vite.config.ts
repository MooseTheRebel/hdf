import {fileURLToPath} from 'node:url';
import {defineConfig} from 'vite';

// The GUI text lives in the repository's top-level locales/ (shared with
// the Vue.js GUI), imported as "@locales".
const locales = fileURLToPath(new URL('../../../locales', import.meta.url));

export default defineConfig({
    resolve: {
        alias: {'@locales': locales},
    },
    server: {
        fs: {
            // Setting allow replaces Vite's default, so this project's own
            // root is listed too.
            allow: ['.', locales],
        },
    },
});
