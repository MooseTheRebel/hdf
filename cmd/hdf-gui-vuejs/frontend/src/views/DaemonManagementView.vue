<script setup lang="ts">
import {ref} from 'vue';
import {fmt, msg} from '@locales';
import {GetDaemonStatus, InstallDaemon, StartDaemon, StopDaemon, UninstallDaemon} from '../../wailsjs/go/cli/App';
import DaemonBadge from '../components/DaemonBadge.vue';

const emit = defineEmits<{back: []}>();

const status = ref<string | null>(null);
const loadError = ref('');
const result = ref('');

function refresh() {
    status.value = null;
    GetDaemonStatus()
        .then((v) => { status.value = v; })
        .catch((err) => { loadError.value = fmt(msg.daemon.loadError, {error: String(err)}); });
}

function runAction(action: () => Promise<void>, successMessage: string) {
    result.value = '';
    action()
        .then(() => {
            result.value = successMessage;
            refresh();
        })
        .catch((err) => { result.value = fmt(msg.daemon.actionError, {error: String(err)}); });
}

refresh();
</script>

<template>
    <div class="daemon-management-container">
        <div class="daemon-management-header-section">
            <h1>{{ msg.daemon.managementTitle }}</h1>
        </div>
        <div v-if="loadError">{{ loadError }}</div>
        <div v-else-if="status === null">{{ msg.daemon.loading }}</div>
        <div v-else>
            <div class="daemon-management-status"><DaemonBadge :status="status"/></div>
            <div class="daemon-management-actions">
                <button id="daemon-install-btn" class="control-btn" @click="runAction(InstallDaemon, msg.daemon.installed)">{{ msg.daemon.install }}</button>
                <button id="daemon-uninstall-btn" class="control-btn" @click="runAction(UninstallDaemon, msg.daemon.uninstalled)">{{ msg.daemon.uninstall }}</button>
                <button id="daemon-start-btn" class="control-btn" @click="runAction(StartDaemon, msg.daemon.started)">{{ msg.daemon.start }}</button>
                <button id="daemon-stop-btn" class="control-btn" @click="runAction(StopDaemon, msg.daemon.stopped)">{{ msg.daemon.stop }}</button>
            </div>
        </div>
        <div id="daemon-management-result">{{ result }}</div>
        <div class="daemon-management-controls">
            <button class="control-btn" @click="emit('back')">{{ msg.common.back }}</button>
        </div>
    </div>
</template>
