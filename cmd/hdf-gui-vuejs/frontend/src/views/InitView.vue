<script setup lang="ts">
import {ref} from 'vue';
import {fmt, msg} from '@locales';
import {DefaultRepoPath, FinishInit, PickDirectory, ResolveBranchCollision, StartInitLocal, StartInitRemote} from '../../wailsjs/go/cli/App';
import type {cli} from '../../wailsjs/go/models';
import StepPage from '../components/StepPage.vue';
import I18nText from '../components/I18nText';

const emit = defineEmits<{back: []}>();

type Step =
    | {kind: 'mode'}
    | {kind: 'busy'; message: string}
    | {kind: 'localForm'}
    | {kind: 'remoteForm'}
    | {kind: 'collision'; branch: string}
    | {kind: 'done'; result: cli.InitResult}
    | {kind: 'error'; message: string};

const step = ref<Step>({kind: 'mode'});

// Form fields.
const repoPath = ref('');
const pushTarget = ref('');
const gitURL = ref('');
const cloneDir = ref('');

function fail(template: string) {
    return (err: unknown) => { step.value = {kind: 'error', message: fmt(template, {error: String(err)})}; };
}

function showForm(kind: 'localForm' | 'remoteForm') {
    step.value = {kind: 'busy', message: msg.init.loadingDefaults};
    DefaultRepoPath()
        .then((defaultPath) => {
            repoPath.value = defaultPath;
            cloneDir.value = defaultPath;
            step.value = {kind};
        })
        .catch(fail(msg.init.defaultsError));
}

const browsable = {repoPath, pushTarget, cloneDir};

function browse(field: keyof typeof browsable) {
    PickDirectory()
        .then((path) => { if (path) browsable[field].value = path; })
        .catch((err) => console.error(fmt(msg.init.pickDirectoryError, {error: String(err)})));
}

function start(begin: () => Promise<cli.InitStartInfo>) {
    step.value = {kind: 'busy', message: msg.init.settingUp};
    begin()
        .then((info) => {
            if (info.collision) step.value = {kind: 'collision', branch: info.collision.branch};
            else finish();
        })
        .catch(fail(msg.init.setupError));
}

function resolveCollision(useUnique: boolean) {
    step.value = {kind: 'busy', message: msg.init.settingUp};
    ResolveBranchCollision(useUnique)
        .then(finish)
        .catch(fail(msg.init.collisionError));
}

function finish() {
    step.value = {kind: 'busy', message: msg.init.finishing};
    FinishInit()
        .then((result) => { step.value = {kind: 'done', result}; })
        .catch(fail(msg.init.finishError));
}
</script>

<template>
    <StepPage :title="msg.init.title">
        <p v-if="step.kind === 'mode'">{{ msg.init.modePrompt }}</p>
        <template v-else-if="step.kind === 'busy' || step.kind === 'error'">{{ step.message }}</template>
        <template v-else-if="step.kind === 'localForm'">
            <label class="init-field-label" for="init-repo-path">{{ msg.init.repoPath }}</label>
            <div class="init-field-row">
                <input type="text" id="init-repo-path" class="init-text-input" v-model="repoPath">
                <button id="init-repo-browse-btn" class="control-btn" @click="browse('repoPath')">{{ msg.common.browse }}</button>
            </div>
            <label class="init-field-label" for="init-push-target">{{ msg.init.pushTarget }}</label>
            <div class="init-field-row">
                <input type="text" id="init-push-target" class="init-text-input" v-model="pushTarget">
                <button id="init-push-browse-btn" class="control-btn" @click="browse('pushTarget')">{{ msg.common.browse }}</button>
            </div>
        </template>
        <template v-else-if="step.kind === 'remoteForm'">
            <label class="init-field-label" for="init-git-url">{{ msg.init.gitUrl }}</label>
            <div class="init-field-row">
                <input type="text" id="init-git-url" class="init-text-input" v-model="gitURL">
            </div>
            <label class="init-field-label" for="init-clone-dir">{{ msg.init.cloneDir }}</label>
            <div class="init-field-row">
                <input type="text" id="init-clone-dir" class="init-text-input" v-model="cloneDir">
                <button id="init-clone-browse-btn" class="control-btn" @click="browse('cloneDir')">{{ msg.common.browse }}</button>
            </div>
        </template>
        <template v-else-if="step.kind === 'collision'">
            <p><I18nText :template="msg.init.collision"><template #branch><strong>{{ step.branch }}</strong></template></I18nText></p>
            <p>{{ msg.init.collisionQuestion }}</p>
        </template>
        <p v-else-if="step.kind === 'done'" class="init-result-message">{{ step.result.message }}</p>

        <template #controls>
            <template v-if="step.kind === 'mode'">
                <button class="control-btn" @click="showForm('localForm')">{{ msg.init.modeLocal }}</button>
                <button class="control-btn" @click="showForm('remoteForm')">{{ msg.init.modeRemote }}</button>
                <button class="control-btn" @click="emit('back')">{{ msg.common.cancel }}</button>
            </template>
            <template v-else-if="step.kind === 'localForm'">
                <button id="init-continue-btn" class="control-btn" @click="start(() => StartInitLocal(repoPath, pushTarget))">{{ msg.common.continue }}</button>
                <button class="control-btn" @click="emit('back')">{{ msg.common.cancel }}</button>
            </template>
            <template v-else-if="step.kind === 'remoteForm'">
                <button id="init-continue-btn" class="control-btn" @click="start(() => StartInitRemote(gitURL, cloneDir))">{{ msg.common.continue }}</button>
                <button class="control-btn" @click="emit('back')">{{ msg.common.cancel }}</button>
            </template>
            <template v-else-if="step.kind === 'collision'">
                <button class="control-btn" @click="resolveCollision(false)">{{ msg.init.reuse }}</button>
                <button class="control-btn" @click="resolveCollision(true)">{{ msg.init.unique }}</button>
            </template>
            <button v-else-if="step.kind === 'done'" class="control-btn" @click="emit('back')">{{ msg.common.continue }}</button>
            <button v-else-if="step.kind === 'error'" class="control-btn" @click="emit('back')">{{ msg.common.back }}</button>
        </template>
    </StepPage>
</template>
