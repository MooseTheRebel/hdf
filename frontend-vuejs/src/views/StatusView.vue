<script setup lang="ts">
import {ref} from 'vue';
import {GetStatus} from '../../wailsjs/go/cli/App';
import type {cli} from '../../wailsjs/go/models';

const emit = defineEmits<{back: []}>();

const info = ref<cli.StatusInfo | null>(null);
const error = ref('');

GetStatus()
    .then((v) => { info.value = v; })
    .catch((err) => { error.value = 'Error loading status: ' + err; });
</script>

<template>
    <div class="status-container">
        <div class="status-header-section">
            <h1>Status</h1>
        </div>
        <div v-if="error">{{ error }}</div>
        <div v-else-if="!info">Loading status...</div>
        <div v-else class="status-content">
            <div class="status-summary">
                <div class="status-field"><span class="status-label">Git push target</span><span class="status-value">{{ info.git_push_target }}</span></div>
                <div class="status-field"><span class="status-label">Local dotfiles dir</span><span class="status-value">{{ info.local_dotfiles_dir }}</span></div>
                <div class="status-field"><span class="status-label">Branch</span><span class="status-value">{{ info.branch }}</span></div>
                <div class="status-field"><span class="status-label">Last commit</span><span class="status-value">{{ info.last_commit }}</span></div>
                <div class="status-field"><span class="status-label">Last sync</span><span class="status-value">{{ info.last_sync }}</span></div>
            </div>
            <h2 class="status-files-heading">Managed files ({{ info.files.length }})</h2>
            <div class="status-file-list">
                <div v-for="f in info.files" :key="f.path" class="status-file-row">
                    <span class="status-file-path">{{ f.path }}</span>
                    <span class="status-file-state">{{ f.status }}</span>
                </div>
                <div v-if="info.files.length === 0" class="status-empty">No managed files.</div>
            </div>
        </div>
        <div class="status-controls">
            <button class="control-btn" @click="emit('back')">Back</button>
        </div>
    </div>
</template>
