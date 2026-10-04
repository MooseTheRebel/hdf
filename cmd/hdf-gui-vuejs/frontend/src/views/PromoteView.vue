<script setup lang="ts">
import {ref} from 'vue';
import {fmt, msg} from '@locales';
import {FinishPromote, ResolveDivergedFile, StartPromote} from '../../wailsjs/go/cli/App';
import type {cli} from '../../wailsjs/go/models';
import StepPage from '../components/StepPage.vue';
import DiffContent from '../components/DiffContent.vue';

const emit = defineEmits<{back: []}>();

type Step =
    | {kind: 'busy'; message: string}
    | {kind: 'preserved'; info: cli.PromoteStartInfo}
    | {kind: 'review'; files: cli.DivergedFile[]; index: number}
    | {kind: 'done'; result: cli.PromoteResult}
    | {kind: 'error'; message: string};

const step = ref<Step>({kind: 'busy', message: msg.promote.checking});

function fail(template: string, params: Record<string, string> = {}) {
    return (err: unknown) => { step.value = {kind: 'error', message: fmt(template, {...params, error: String(err)})}; };
}

function reviewOrFinish(diverged: cli.DivergedFile[]) {
    if (diverged.length > 0) step.value = {kind: 'review', files: diverged, index: 0};
    else finish();
}

function resolve(files: cli.DivergedFile[], index: number, keepMine: boolean) {
    step.value = {kind: 'busy', message: ''};
    ResolveDivergedFile(index, keepMine)
        .then(() => {
            if (index + 1 < files.length) step.value = {kind: 'review', files, index: index + 1};
            else finish();
        })
        .catch(fail(msg.promote.resolveError, {path: files[index].path}));
}

function finish() {
    step.value = {kind: 'busy', message: msg.promote.promoting};
    FinishPromote()
        .then((result) => { step.value = {kind: 'done', result}; })
        .catch(fail(msg.promote.error));
}

StartPromote()
    .then((info) => {
        if (info.preserved.length > 0) step.value = {kind: 'preserved', info};
        else reviewOrFinish(info.diverged);
    })
    .catch(fail(msg.promote.startError));
</script>

<template>
    <StepPage :title="msg.promote.title">
        <template v-if="step.kind === 'busy' || step.kind === 'error'">{{ step.message }}</template>
        <template v-else-if="step.kind === 'preserved'">
            <p>{{ msg.promote.preservedIntro }}</p>
            <div class="link-warning-list">
                <div v-for="f in step.info.preserved" :key="f.path" class="link-warning-row">{{ f.path }}</div>
            </div>
            <p>{{ msg.promote.preservedPrompt }}</p>
        </template>
        <template v-else-if="step.kind === 'review'">
            <div class="link-review-counter">{{ fmt(msg.common.reviewCounter, {index: step.index + 1, total: step.files.length}) }}</div>
            <div class="link-review-path">{{ step.files[step.index].path }}</div>
            <div class="link-review-diff"><DiffContent :content="step.files[step.index].diff"/></div>
        </template>
        <p v-else-if="step.kind === 'done'" class="init-result-message">{{ step.result.message }}</p>

        <template #controls>
            <template v-if="step.kind === 'preserved'">
                <button class="control-btn" @click="reviewOrFinish(step.info.diverged)">{{ msg.common.continue }}</button>
                <button class="control-btn" @click="emit('back')">{{ msg.common.cancel }}</button>
            </template>
            <template v-else-if="step.kind === 'review'">
                <button id="promote-keep-mine-btn" class="control-btn" @click="resolve(step.files, step.index, true)">{{ msg.promote.keepMine }}</button>
                <button id="promote-keep-theirs-btn" class="control-btn" @click="resolve(step.files, step.index, false)">{{ msg.promote.keepTheirs }}</button>
            </template>
            <button v-else-if="step.kind === 'done'" class="control-btn" @click="emit('back')">{{ msg.common.continue }}</button>
            <button v-else-if="step.kind === 'error'" class="control-btn" @click="emit('back')">{{ msg.common.back }}</button>
        </template>
    </StepPage>
</template>
