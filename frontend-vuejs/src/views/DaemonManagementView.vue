<script setup lang="ts">
import {ref} from 'vue';
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
        .catch((err) => { loadError.value = 'Error loading daemon status: ' + err; });
}

function runAction(action: () => Promise<void>, successMessage: string) {
    result.value = '';
    action()
        .then(() => {
            result.value = successMessage;
            refresh();
        })
        .catch((err) => { result.value = 'Error: ' + err; });
}

refresh();
</script>

<template>
    <div class="daemon-management-container">
        <div class="daemon-management-header-section">
            <h1>Daemon Management</h1>
        </div>
        <div v-if="loadError">{{ loadError }}</div>
        <div v-else-if="status === null">Loading daemon status...</div>
        <div v-else>
            <div class="daemon-management-status"><DaemonBadge :status="status"/></div>
            <div class="daemon-management-actions">
                <button id="daemon-install-btn" class="control-btn" @click="runAction(InstallDaemon, 'Daemon installed and started.')">Install</button>
                <button id="daemon-uninstall-btn" class="control-btn" @click="runAction(UninstallDaemon, 'Daemon uninstalled.')">Uninstall</button>
                <button id="daemon-start-btn" class="control-btn" @click="runAction(StartDaemon, 'Daemon started.')">Start</button>
                <button id="daemon-stop-btn" class="control-btn" @click="runAction(StopDaemon, 'Daemon stopped.')">Stop</button>
            </div>
        </div>
        <div id="daemon-management-result">{{ result }}</div>
        <div class="daemon-management-controls">
            <button class="control-btn" @click="emit('back')">Back</button>
        </div>
    </div>
</template>
