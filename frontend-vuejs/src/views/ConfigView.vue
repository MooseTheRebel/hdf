<script setup lang="ts">
import {ref} from 'vue';
import {GetConfig} from '../../wailsjs/go/cli/App';
import type {cli} from '../../wailsjs/go/models';

const emit = defineEmits<{back: []}>();

const info = ref<cli.ConfigInfo | null>(null);
const error = ref('');

GetConfig()
    .then((v) => { info.value = v; })
    .catch((err) => { error.value = 'Error loading config: ' + err; });
</script>

<template>
    <div class="config-container">
        <div class="config-header-section">
            <h1>Config</h1>
        </div>
        <div v-if="error">{{ error }}</div>
        <div v-else-if="!info">Loading config...</div>
        <p v-else-if="!info.exists" class="config-missing">No config found. Run <code>hdf init</code> to get started.</p>
        <div v-else class="config-details">
            <div class="config-field"><span class="config-label">Config file</span><span class="config-value">{{ info.path }}</span></div>
            <pre class="config-content">{{ info.content }}</pre>
        </div>
        <div class="config-controls">
            <button class="control-btn" @click="emit('back')">Back</button>
        </div>
    </div>
</template>
