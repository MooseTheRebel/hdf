<script setup lang="ts">
import {ref} from 'vue';
import {SubmitReportIssue} from '../../wailsjs/go/cli/App';
import type {cli} from '../../wailsjs/go/models';
import StepPage from '../components/StepPage.vue';

const emit = defineEmits<{back: []}>();

const reportIssueWarning = "Warning: hdf's automatic redaction is limited — it strips a few known " +
    'credential patterns, but this is not a guarantee that all sensitive information is removed. ' +
    "Review the report's contents yourself before sharing it with anyone. " +
    'Reporting an issue is entirely optional and voluntary. Securing your own systems should ' +
    'always be your first priority.';

type Step =
    | {kind: 'form'}
    | {kind: 'busy'}
    | {kind: 'done'; result: cli.ReportIssueResult}
    | {kind: 'error'; message: string};

const step = ref<Step>({kind: 'form'});
const text = ref('');

function submit() {
    step.value = {kind: 'busy'};
    SubmitReportIssue(text.value)
        .then((result) => { step.value = {kind: 'done', result}; })
        .catch((err) => { step.value = {kind: 'error', message: 'Error building report: ' + err}; });
}
</script>

<template>
    <StepPage title="Report Issue">
        <template v-if="step.kind === 'form'">
            <p>{{ reportIssueWarning }}</p>
            <label class="init-field-label" for="report-issue-text">What was expected? What actually happened? (optional)</label>
            <textarea id="report-issue-text" class="init-text-input report-issue-textarea" v-model="text"></textarea>
        </template>
        <template v-else-if="step.kind === 'busy'">Building report...</template>
        <p v-else-if="step.kind === 'done'" class="init-result-message">Report written to <code>{{ step.result.path }}</code></p>
        <template v-else>{{ step.message }}</template>

        <template #controls>
            <template v-if="step.kind === 'form'">
                <button id="report-issue-submit-btn" class="control-btn" @click="submit">Build Report</button>
                <button class="control-btn" @click="emit('back')">Cancel</button>
            </template>
            <button v-else-if="step.kind !== 'busy'" class="control-btn" @click="emit('back')">Back</button>
        </template>
    </StepPage>
</template>
