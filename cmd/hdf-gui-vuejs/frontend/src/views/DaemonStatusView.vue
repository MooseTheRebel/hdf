<script setup lang="ts">
import {ref} from 'vue';
import {fmt, msg} from '@locales';
import {GetDaemonStatus} from '../../wailsjs/go/cli/App';
import DaemonBadge from '../components/DaemonBadge.vue';

const emit = defineEmits<{back: []}>();

const status = ref<string | null>(null);
const error = ref('');

GetDaemonStatus()
    .then((v) => { status.value = v; })
    .catch((err) => { error.value = fmt(msg.daemon.loadError, {error: String(err)}); });
</script>

<template>
    <div class="daemon-status-container">
        <div class="daemon-status-header-section">
            <h1>{{ msg.daemon.statusTitle }}</h1>
        </div>
        <div v-if="error">{{ error }}</div>
        <div v-else-if="status === null">{{ msg.daemon.loading }}</div>
        <div v-else><DaemonBadge :status="status"/></div>
        <div class="daemon-status-controls">
            <button class="control-btn" @click="emit('back')">{{ msg.common.back }}</button>
        </div>
    </div>
</template>
