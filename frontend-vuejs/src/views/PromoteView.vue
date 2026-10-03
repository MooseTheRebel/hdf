<script setup lang="ts">
import {ref} from 'vue';
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

const step = ref<Step>({kind: 'busy', message: 'Checking for changes to promote...'});

function fail(prefix: string) {
    return (err: unknown) => { step.value = {kind: 'error', message: prefix + err}; };
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
        .catch(fail('Error resolving ' + files[index].path + ': '));
}

function finish() {
    step.value = {kind: 'busy', message: 'Promoting...'};
    FinishPromote()
        .then((result) => { step.value = {kind: 'done', result}; })
        .catch(fail('Error promoting: '));
}

StartPromote()
    .then((info) => {
        if (info.preserved.length > 0) step.value = {kind: 'preserved', info};
        else reviewOrFinish(info.diverged);
    })
    .catch(fail('Error starting promote: '));
</script>

<template>
    <StepPage title="Promote">
        <template v-if="step.kind === 'busy' || step.kind === 'error'">{{ step.message }}</template>
        <template v-else-if="step.kind === 'preserved'">
            <p>main has file(s) promoted by other machines that you haven't pulled. They will be preserved by promote:</p>
            <div class="link-warning-list">
                <div v-for="f in step.info.preserved" :key="f.path" class="link-warning-row">{{ f.path }}</div>
            </div>
            <p>Continue promoting?</p>
        </template>
        <template v-else-if="step.kind === 'review'">
            <div class="link-review-counter">File {{ step.index + 1 }} of {{ step.files.length }}</div>
            <div class="link-review-path">{{ step.files[step.index].path }}</div>
            <div class="link-review-diff"><DiffContent :content="step.files[step.index].diff"/></div>
        </template>
        <p v-else-if="step.kind === 'done'" class="init-result-message">{{ step.result.message }}</p>

        <template #controls>
            <template v-if="step.kind === 'preserved'">
                <button class="control-btn" @click="reviewOrFinish(step.info.diverged)">Continue</button>
                <button class="control-btn" @click="emit('back')">Cancel</button>
            </template>
            <template v-else-if="step.kind === 'review'">
                <button id="promote-keep-mine-btn" class="control-btn" @click="resolve(step.files, step.index, true)">Overwrite main with mine</button>
                <button id="promote-keep-theirs-btn" class="control-btn" @click="resolve(step.files, step.index, false)">Keep main's version</button>
            </template>
            <button v-else-if="step.kind === 'done'" class="control-btn" @click="emit('back')">Continue</button>
            <button v-else-if="step.kind === 'error'" class="control-btn" @click="emit('back')">Back</button>
        </template>
    </StepPage>
</template>
