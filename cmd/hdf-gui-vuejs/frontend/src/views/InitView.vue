<script setup lang="ts">
import {ref} from 'vue';
import {DefaultRepoPath, FinishInit, PickDirectory, ResolveBranchCollision, StartInitLocal, StartInitRemote} from '../../wailsjs/go/cli/App';
import type {cli} from '../../wailsjs/go/models';
import StepPage from '../components/StepPage.vue';

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

function fail(prefix: string) {
    return (err: unknown) => { step.value = {kind: 'error', message: prefix + err}; };
}

function showForm(kind: 'localForm' | 'remoteForm') {
    step.value = {kind: 'busy', message: 'Loading default path...'};
    DefaultRepoPath()
        .then((defaultPath) => {
            repoPath.value = defaultPath;
            cloneDir.value = defaultPath;
            step.value = {kind};
        })
        .catch(fail('Error loading default path: '));
}

const browsable = {repoPath, pushTarget, cloneDir};

function browse(field: keyof typeof browsable) {
    PickDirectory()
        .then((path) => { if (path) browsable[field].value = path; })
        .catch((err) => console.error('Error picking a directory: ' + err));
}

function start(begin: () => Promise<cli.InitStartInfo>) {
    step.value = {kind: 'busy', message: 'Setting up...'};
    begin()
        .then((info) => {
            if (info.collision) step.value = {kind: 'collision', branch: info.collision.branch};
            else finish();
        })
        .catch(fail('Error setting up: '));
}

function resolveCollision(useUnique: boolean) {
    step.value = {kind: 'busy', message: 'Setting up...'};
    ResolveBranchCollision(useUnique)
        .then(finish)
        .catch(fail('Error resolving branch collision: '));
}

function finish() {
    step.value = {kind: 'busy', message: 'Finishing setup...'};
    FinishInit()
        .then((result) => { step.value = {kind: 'done', result}; })
        .catch(fail('Error finishing setup: '));
}
</script>

<template>
    <StepPage title="Get Started">
        <p v-if="step.kind === 'mode'">How do you want to store your dot files?</p>
        <template v-else-if="step.kind === 'busy' || step.kind === 'error'">{{ step.message }}</template>
        <template v-else-if="step.kind === 'localForm'">
            <label class="init-field-label" for="init-repo-path">Local repo path</label>
            <div class="init-field-row">
                <input type="text" id="init-repo-path" class="init-text-input" v-model="repoPath">
                <button id="init-repo-browse-btn" class="control-btn" @click="browse('repoPath')">Browse...</button>
            </div>
            <label class="init-field-label" for="init-push-target">Push target path or remote URL (optional)</label>
            <div class="init-field-row">
                <input type="text" id="init-push-target" class="init-text-input" v-model="pushTarget">
                <button id="init-push-browse-btn" class="control-btn" @click="browse('pushTarget')">Browse...</button>
            </div>
        </template>
        <template v-else-if="step.kind === 'remoteForm'">
            <label class="init-field-label" for="init-git-url">Remote repository URL</label>
            <div class="init-field-row">
                <input type="text" id="init-git-url" class="init-text-input" v-model="gitURL">
            </div>
            <label class="init-field-label" for="init-clone-dir">Clone destination</label>
            <div class="init-field-row">
                <input type="text" id="init-clone-dir" class="init-text-input" v-model="cloneDir">
                <button id="init-clone-browse-btn" class="control-btn" @click="browse('cloneDir')">Browse...</button>
            </div>
        </template>
        <template v-else-if="step.kind === 'collision'">
            <p>A branch named <strong>{{ step.branch }}</strong> already exists on the remote.</p>
            <p>Is this machine re-initializing (reuse it), or is this a different machine that happens to share this name (create a unique branch)?</p>
        </template>
        <p v-else-if="step.kind === 'done'" class="init-result-message">{{ step.result.message }}</p>

        <template #controls>
            <template v-if="step.kind === 'mode'">
                <button class="control-btn" @click="showForm('localForm')">Local directory</button>
                <button class="control-btn" @click="showForm('remoteForm')">Remote repository</button>
                <button class="control-btn" @click="emit('back')">Cancel</button>
            </template>
            <template v-else-if="step.kind === 'localForm'">
                <button id="init-continue-btn" class="control-btn" @click="start(() => StartInitLocal(repoPath, pushTarget))">Continue</button>
                <button class="control-btn" @click="emit('back')">Cancel</button>
            </template>
            <template v-else-if="step.kind === 'remoteForm'">
                <button id="init-continue-btn" class="control-btn" @click="start(() => StartInitRemote(gitURL, cloneDir))">Continue</button>
                <button class="control-btn" @click="emit('back')">Cancel</button>
            </template>
            <template v-else-if="step.kind === 'collision'">
                <button class="control-btn" @click="resolveCollision(false)">Reuse it</button>
                <button class="control-btn" @click="resolveCollision(true)">Create a unique branch</button>
            </template>
            <button v-else-if="step.kind === 'done'" class="control-btn" @click="emit('back')">Continue</button>
            <button v-else-if="step.kind === 'error'" class="control-btn" @click="emit('back')">Back</button>
        </template>
    </StepPage>
</template>
