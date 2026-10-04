<script setup lang="ts">
import {ref} from 'vue';
import {fmt, msg} from '@locales';
import {AcceptIncomingFile, FinishLink, GetPendingWarnings, StartLink} from '../../wailsjs/go/cli/App';
import type {cli} from '../../wailsjs/go/models';
import StepPage from '../components/StepPage.vue';
import DiffContent from '../components/DiffContent.vue';

const props = defineProps<{noFetch: boolean}>();
const emit = defineEmits<{back: []}>();

type Step =
    | {kind: 'busy'; message: string}
    | {kind: 'warnings'; warnings: string[]}
    | {kind: 'review'; files: cli.IncomingFile[]; index: number}
    | {kind: 'acceptError'; files: cli.IncomingFile[]; index: number; error: string}
    | {kind: 'done'; message: string; results: cli.LinkedFile[]}
    | {kind: 'error'; message: string};

const step = ref<Step>({kind: 'busy', message: msg.warnings.checking});

function fail(template: string) {
    return (err: unknown) => { step.value = {kind: 'error', message: fmt(template, {error: String(err)})}; };
}

function checkWarnings() {
    GetPendingWarnings()
        .then((warnings) => {
            if (warnings.length > 0) step.value = {kind: 'warnings', warnings};
            else start();
        })
        .catch(fail(msg.warnings.checkError));
}

function start() {
    step.value = {kind: 'busy', message: props.noFetch ? msg.link.skippingFetch : msg.link.fetching};
    StartLink(props.noFetch)
        .then((info) => {
            if (info.incomingFiles.length > 0) step.value = {kind: 'review', files: info.incomingFiles, index: 0};
            else finish(info.message);
        })
        .catch(fail(msg.link.startError));
}

function accept(files: cli.IncomingFile[], index: number) {
    step.value = {kind: 'busy', message: ''};
    AcceptIncomingFile(index)
        .then(() => advance(files, index))
        .catch((err) => { step.value = {kind: 'acceptError', files, index, error: String(err)}; });
}

function advance(files: cli.IncomingFile[], index: number) {
    if (index + 1 < files.length) step.value = {kind: 'review', files, index: index + 1};
    else finish('');
}

function finish(message: string) {
    step.value = {kind: 'busy', message: msg.link.relinking};
    FinishLink()
        .then((results) => { step.value = {kind: 'done', message, results}; })
        .catch(fail(msg.link.finishError));
}

checkWarnings();
</script>

<template>
    <StepPage :title="msg.link.title" content-id="link-step-content">
        <template v-if="step.kind === 'busy' || step.kind === 'error'">{{ step.message }}</template>
        <template v-else-if="step.kind === 'warnings'">
            <p>{{ msg.warnings.intro }}</p>
            <div class="link-warning-list">
                <div v-for="(w, i) in step.warnings" :key="i" class="link-warning-row">{{ w }}</div>
            </div>
            <p>{{ msg.warnings.prompt }}</p>
        </template>
        <template v-else-if="step.kind === 'review'">
            <div class="link-review-counter">{{ fmt(msg.common.reviewCounter, {index: step.index + 1, total: step.files.length}) }}</div>
            <div class="link-review-path">{{ step.files[step.index].path }}</div>
            <div class="link-review-diff"><DiffContent :content="step.files[step.index].diff"/></div>
        </template>
        <p v-else-if="step.kind === 'acceptError'" class="link-error">{{ fmt(msg.link.acceptError, {path: step.files[step.index].path, error: step.error}) }}</p>
        <template v-else-if="step.kind === 'done'">
            <p v-if="step.message" class="link-results-message">{{ step.message }}</p>
            <div class="link-results-list">
                <div v-for="r in step.results" :key="r.path" class="link-result-row" :class="r.error ? 'link-result-error' : 'link-result-ok'">
                    <span class="link-result-path">{{ r.path }}</span>
                    <span class="link-result-status">{{ r.error || msg.link.linked }}</span>
                </div>
                <div v-if="step.results.length === 0" class="link-results-empty">{{ msg.common.noManagedFiles }}</div>
            </div>
        </template>

        <template #controls>
            <template v-if="step.kind === 'warnings'">
                <button class="control-btn" @click="start">{{ msg.common.continue }}</button>
                <button class="control-btn" @click="emit('back')">{{ msg.common.cancel }}</button>
            </template>
            <template v-else-if="step.kind === 'review'">
                <button id="link-accept-btn" class="control-btn" @click="accept(step.files, step.index)">{{ msg.link.accept }}</button>
                <button id="link-skip-btn" class="control-btn" @click="advance(step.files, step.index)">{{ msg.link.skip }}</button>
            </template>
            <template v-else-if="step.kind === 'acceptError'">
                <button class="control-btn" @click="advance(step.files, step.index)">{{ msg.link.skipAndContinue }}</button>
                <button class="control-btn" @click="emit('back')">{{ msg.common.cancel }}</button>
            </template>
            <button v-else-if="step.kind === 'done' || step.kind === 'error'" class="control-btn" @click="emit('back')">{{ msg.common.back }}</button>
        </template>
    </StepPage>
</template>
