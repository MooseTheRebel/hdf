<script setup lang="ts">
import {ref} from 'vue';
import {ConfirmEnroll, GetPendingWarnings, StartEnroll} from '../../wailsjs/go/cli/App';
import type {cli} from '../../wailsjs/go/models';
import StepPage from '../components/StepPage.vue';
import DiffContent from '../components/DiffContent.vue';

const props = defineProps<{path: string; pickError?: string}>();
const emit = defineEmits<{back: []}>();

type Step =
    | {kind: 'busy'; message: string}
    | {kind: 'warnings'; warnings: string[]}
    | {kind: 'preview'; info: cli.EnrollStartInfo}
    | {kind: 'done'; result: cli.EnrollResult}
    | {kind: 'error'; message: string};

const step = ref<Step>({kind: 'busy', message: 'Checking for pending warnings...'});

function fail(prefix: string) {
    return (err: unknown) => { step.value = {kind: 'error', message: prefix + err}; };
}

function checkWarnings() {
    GetPendingWarnings()
        .then((warnings) => {
            if (warnings.length > 0) step.value = {kind: 'warnings', warnings};
            else start();
        })
        .catch(fail('Error checking pending warnings: '));
}

function start() {
    step.value = {kind: 'busy', message: 'Preparing enroll preview...'};
    StartEnroll(props.path)
        .then((info) => { step.value = {kind: 'preview', info}; })
        .catch(fail('Error starting enroll: '));
}

function confirm() {
    step.value = {kind: 'busy', message: 'Enrolling...'};
    ConfirmEnroll()
        .then((result) => { step.value = {kind: 'done', result}; })
        .catch(fail('Error enrolling: '));
}

if (props.pickError) step.value = {kind: 'error', message: props.pickError};
else checkWarnings();
</script>

<template>
    <StepPage title="Enroll">
        <template v-if="step.kind === 'busy' || step.kind === 'error'">{{ step.message }}</template>
        <template v-else-if="step.kind === 'warnings'">
            <p>The hdf daemon has recorded the following warnings:</p>
            <div class="link-warning-list">
                <div v-for="(w, i) in step.warnings" :key="i" class="link-warning-row">{{ w }}</div>
            </div>
            <p>Continue anyway?</p>
        </template>
        <template v-else-if="step.kind === 'preview'">
            <div class="enroll-preview-path">{{ step.info.path }}</div>
            <p v-if="step.info.isNewFile" class="enroll-preview-note">This is a new file.</p>
            <p v-else-if="!step.info.diff" class="enroll-preview-note">There are no changes to enroll.</p>
            <div v-else class="enroll-preview-diff"><DiffContent :content="step.info.diff"/></div>
        </template>
        <p v-else-if="step.kind === 'done'" class="enroll-result-message">{{ step.result.message }}</p>

        <template #controls>
            <template v-if="step.kind === 'warnings'">
                <button class="control-btn" @click="start">Continue</button>
                <button class="control-btn" @click="emit('back')">Cancel</button>
            </template>
            <template v-else-if="step.kind === 'preview'">
                <button class="control-btn" @click="confirm">Enroll</button>
                <button class="control-btn" @click="emit('back')">Cancel</button>
            </template>
            <button v-else-if="step.kind === 'done' || step.kind === 'error'" class="control-btn" @click="emit('back')">Back</button>
        </template>
    </StepPage>
</template>
