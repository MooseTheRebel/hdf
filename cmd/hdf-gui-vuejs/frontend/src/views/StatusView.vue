<script setup lang="ts">
import {ref} from 'vue';
import {fmt, msg} from '@locales';
import {GetStatus} from '../../wailsjs/go/cli/App';
import type {cli} from '../../wailsjs/go/models';

const emit = defineEmits<{back: []}>();

const info = ref<cli.StatusInfo | null>(null);
const error = ref('');

GetStatus()
    .then((v) => { info.value = v; })
    .catch((err) => { error.value = fmt(msg.status.loadError, {error: String(err)}); });
</script>

<template>
    <div class="status-container">
        <div class="status-header-section">
            <h1>{{ msg.status.title }}</h1>
        </div>
        <div v-if="error">{{ error }}</div>
        <div v-else-if="!info">{{ msg.status.loading }}</div>
        <div v-else class="status-content">
            <div class="status-summary">
                <div class="status-field"><span class="status-label">{{ msg.status.gitPushTarget }}</span><span class="status-value">{{ info.git_push_target }}</span></div>
                <div class="status-field"><span class="status-label">{{ msg.status.localDotfilesDir }}</span><span class="status-value">{{ info.local_dotfiles_dir }}</span></div>
                <div class="status-field"><span class="status-label">{{ msg.status.branch }}</span><span class="status-value">{{ info.branch }}</span></div>
                <div class="status-field"><span class="status-label">{{ msg.status.lastCommit }}</span><span class="status-value">{{ info.last_commit }}</span></div>
                <div class="status-field"><span class="status-label">{{ msg.status.lastSync }}</span><span class="status-value">{{ info.last_sync }}</span></div>
            </div>
            <h2 class="status-files-heading">{{ fmt(msg.status.filesHeading, {count: info.files.length}) }}</h2>
            <div class="status-file-list">
                <div v-for="f in info.files" :key="f.path" class="status-file-row">
                    <span class="status-file-path">{{ f.path }}</span>
                    <span class="status-file-state">{{ f.status }}</span>
                </div>
                <div v-if="info.files.length === 0" class="status-empty">{{ msg.common.noManagedFiles }}</div>
            </div>
        </div>
        <div class="status-controls">
            <button class="control-btn" @click="emit('back')">{{ msg.common.back }}</button>
        </div>
    </div>
</template>
