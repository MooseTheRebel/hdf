<script setup lang="ts">
import {ref} from 'vue';
import {fmt, msg} from '@locales';
import {SubmitReportIssue} from '../../wailsjs/go/cli/App';
import type {cli} from '../../wailsjs/go/models';
import StepPage from '../components/StepPage.vue';
import I18nText from '../components/I18nText';

const emit = defineEmits<{back: []}>();

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
        .catch((err) => { step.value = {kind: 'error', message: fmt(msg.reportIssue.error, {error: String(err)})}; });
}
</script>

<template>
    <StepPage :title="msg.reportIssue.title">
        <template v-if="step.kind === 'form'">
            <p>{{ msg.reportIssue.warning }}</p>
            <label class="init-field-label" for="report-issue-text">{{ msg.reportIssue.prompt }}</label>
            <textarea id="report-issue-text" class="init-text-input report-issue-textarea" v-model="text"></textarea>
        </template>
        <template v-else-if="step.kind === 'busy'">{{ msg.reportIssue.building }}</template>
        <p v-else-if="step.kind === 'done'" class="init-result-message">
            <I18nText :template="msg.reportIssue.written"><template #path><code>{{ step.result.path }}</code></template></I18nText>
        </p>
        <template v-else>{{ step.message }}</template>

        <template #controls>
            <template v-if="step.kind === 'form'">
                <button id="report-issue-submit-btn" class="control-btn" @click="submit">{{ msg.reportIssue.submit }}</button>
                <button class="control-btn" @click="emit('back')">{{ msg.common.cancel }}</button>
            </template>
            <button v-else-if="step.kind !== 'busy'" class="control-btn" @click="emit('back')">{{ msg.common.back }}</button>
        </template>
    </StepPage>
</template>
