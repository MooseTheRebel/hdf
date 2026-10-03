// The dark theme and its fonts are the vanilla GUI's, imported rather than
// copied so both GUIs look the same; templates reuse its class names.
import '../../frontend/src/style.css';
import '../../frontend/src/app.css';

import {createApp} from 'vue';
import App from './App.vue';

createApp(App).mount('#app');
