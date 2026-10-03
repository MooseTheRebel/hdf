<script setup lang="ts">
import {ref} from 'vue';
import {CloseWindow, IsInitialized, PickFileToEnroll} from '../../wailsjs/go/cli/App';
import type {View} from '../view';

const emit = defineEmits<{navigate: [view: View]}>();

// null while loading.
const initialized = ref<boolean | null>(null);
const error = ref('');

IsInitialized()
    .then((v) => { initialized.value = v; })
    .catch((err) => { error.value = String(err); });

function startEnroll() {
    PickFileToEnroll()
        .then((path) => { if (path) emit('navigate', {name: 'enroll', path}); })
        .catch((err) => emit('navigate', {name: 'enroll', path: '', pickError: 'Error picking a file: ' + err}));
}
</script>

<template>
    <div v-if="error" class="home-container">
        <div class="home-header">
            <h1 class="home-title">home-dawt-files</h1>
            <span class="home-badge not-initialized">error</span>
        </div>
        <p class="home-subtitle">Could not read hdf configuration.</p>
        <p class="home-subtitle" id="error-message">{{ error }}</p>
        <button class="close-button" @click="CloseWindow()">Close</button>
    </div>

    <div v-else-if="initialized" class="home-container">
        <div class="home-header">
            <h1 class="home-title">home-dawt-files</h1>
            <span class="home-badge initialized">initialized</span>
        </div>
        <p class="home-subtitle">Your dotfiles are managed by hdf.</p>
        <div class="command-list">
            <button class="command-row command-row-clickable" id="enroll-btn" @click="startEnroll">
                <code class="cmd">hdf enroll &lt;path&gt;</code>
                <span class="cmd-desc">Start managing a new dotfile</span>
            </button>
            <button class="command-row command-row-clickable" id="link-btn" @click="emit('navigate', {name: 'link', noFetch: false})">
                <code class="cmd">hdf link</code>
                <span class="cmd-desc">Re-create all managed symlinks</span>
            </button>
            <button class="command-row command-row-clickable" id="link-no-fetch-btn" @click="emit('navigate', {name: 'link', noFetch: true})">
                <code class="cmd">hdf link --no-fetch</code>
                <span class="cmd-desc">Re-create symlinks without fetching from remote</span>
            </button>
            <button class="command-row command-row-clickable" id="promote-btn" @click="emit('navigate', {name: 'promote'})">
                <code class="cmd">hdf promote</code>
                <span class="cmd-desc">Merge your machine branch into main and push</span>
            </button>
            <button class="command-row command-row-clickable" id="status-btn" @click="emit('navigate', {name: 'status'})">
                <code class="cmd">hdf status</code>
                <span class="cmd-desc">Show managed files and sync state</span>
            </button>
            <div class="command-row">
                <code class="cmd">hdf daemon</code>
                <span class="cmd-desc">Start the background sync daemon</span>
            </div>
            <div class="command-row">
                <code class="cmd">hdf diff [url]</code>
                <span class="cmd-desc">View a diff in this window</span>
            </div>
            <button class="command-row command-row-clickable" id="config-btn" @click="emit('navigate', {name: 'config'})">
                <code class="cmd">hdf config</code>
                <span class="cmd-desc">Show the current configuration</span>
            </button>
            <button class="command-row command-row-clickable" id="daemon-status-btn" @click="emit('navigate', {name: 'daemonStatus'})">
                <code class="cmd">hdf daemon status</code>
                <span class="cmd-desc">Check whether the sync daemon service is running</span>
            </button>
            <button class="command-row command-row-clickable" id="daemon-management-btn" @click="emit('navigate', {name: 'daemonManagement'})">
                <code class="cmd">hdf daemon install/start/stop/uninstall</code>
                <span class="cmd-desc">Manage the sync daemon background service</span>
            </button>
            <button class="command-row command-row-clickable" id="report-issue-btn" @click="emit('navigate', {name: 'reportIssue'})">
                <code class="cmd">hdf report-issue</code>
                <span class="cmd-desc">Package diagnostics into a .zip for sharing with an admin</span>
            </button>
        </div>
        <button class="close-button" id="close-btn" @click="CloseWindow()">Close</button>
    </div>

    <div v-else-if="initialized === false" class="home-container">
        <div class="home-header">
            <h1 class="home-title">home-dawt-files</h1>
            <span class="home-badge not-initialized">not initialized</span>
        </div>
        <p class="home-subtitle">Manage your dotfiles with git — across every machine.</p>
        <div class="steps">
            <div class="step">
                <span class="step-number">1</span>
                <div class="step-body">
                    <div class="step-label">Initialize hdf</div>
                    <code class="step-cmd">hdf init</code>
                    <div class="step-hint">Sets up a local git repo and push target.</div>
                </div>
            </div>
            <div class="step">
                <span class="step-number">2</span>
                <div class="step-body">
                    <div class="step-label">Enroll a dotfile</div>
                    <code class="step-cmd">hdf enroll ~/.bashrc</code>
                    <div class="step-hint">Copies the file into the repo and replaces it with a symlink.</div>
                </div>
            </div>
            <div class="step">
                <span class="step-number">3</span>
                <div class="step-body">
                    <div class="step-label">On a new machine — re-link</div>
                    <code class="step-cmd">hdf link</code>
                    <div class="step-hint">Recreates symlinks for all managed files after cloning.</div>
                </div>
            </div>
            <div class="step">
                <span class="step-number">4</span>
                <div class="step-body">
                    <div class="step-label">Check drift</div>
                    <code class="step-cmd">hdf status</code>
                    <div class="step-hint">Shows which files have uncommitted local changes.</div>
                </div>
            </div>
        </div>
        <button class="control-btn" id="get-started-btn" @click="emit('navigate', {name: 'init'})">Get Started</button>
        <button class="close-button" id="close-btn" @click="CloseWindow()">Close</button>
    </div>
</template>
