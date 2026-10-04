<script setup lang="ts">
import {ref} from 'vue';
import {CloseWindow, GetCurrentIndex, GetDiffContent, GetTotalDiffs, NextDiff, PreviousDiff} from '../../wailsjs/go/cli/App';
import DiffContent from '../components/DiffContent.vue';

// null while loading.
const content = ref<string | null>(null);
const error = ref('');
const index = ref(0);
const total = ref(0);
const navigating = ref(false);

function load() {
    content.value = null;
    error.value = '';
    GetDiffContent()
        .then((c) => { content.value = c; })
        .catch((err) => { error.value = 'Error loading diff: ' + err; })
        .finally(updateNavigation);
}

function updateNavigation() {
    Promise.all([GetCurrentIndex(), GetTotalDiffs()]).then(([i, t]) => {
        index.value = i;
        total.value = t;
        navigating.value = false;
    });
}

function move(step: () => Promise<void>) {
    navigating.value = true;
    step().then(load);
}

load();
</script>

<template>
    <div class="diff-container">
        <div class="diff-header-section">
            <h1>Diff Viewer</h1>
            <div class="diff-counter">{{ total ? `Diff ${index + 1} of ${total}` : '' }}</div>
        </div>
        <div v-if="error" id="loading">{{ error }}</div>
        <div v-else-if="content === null" id="loading">Loading diff...</div>
        <div v-else id="diff-content"><DiffContent :content="content"/></div>
        <div class="diff-controls">
            <button id="prev-btn" class="control-btn" :disabled="navigating || index === 0" @click="move(PreviousDiff)">Previous</button>
            <button id="next-btn" class="control-btn" :disabled="navigating || index === total - 1" @click="move(NextDiff)">Next</button>
            <button id="close-btn" class="control-btn close-btn" @click="CloseWindow()">Close</button>
        </div>
    </div>
</template>
