<script setup lang="ts">
import {ref} from 'vue';
import {fmt, msg} from '@locales';
import {CloseWindow, IsInitialized, PickFileToEnroll} from '../../wailsjs/go/cli/App';
import type {View} from '../view';
import CommandRow from '../components/CommandRow.vue';

const emit = defineEmits<{navigate: [view: View]}>();

const c = msg.home.commands;

// null while loading.
const initialized = ref<boolean | null>(null);
const error = ref('');

IsInitialized()
    .then((v) => { initialized.value = v; })
    .catch((err) => { error.value = String(err); });

function startEnroll() {
    PickFileToEnroll()
        .then((path) => { if (path) emit('navigate', {name: 'enroll', path}); })
        .catch((err) => emit('navigate', {name: 'enroll', path: '', pickError: fmt(msg.enroll.pickError, {error: String(err)})}));
}
</script>

<template>
    <div v-if="error" class="home-container">
        <div class="home-header">
            <h1 class="home-title">{{ msg.home.title }}</h1>
            <span class="home-badge not-initialized">{{ msg.home.badgeError }}</span>
        </div>
        <p class="home-subtitle">{{ msg.home.configError }}</p>
        <p class="home-subtitle" id="error-message">{{ error }}</p>
        <button class="close-button" @click="CloseWindow()">{{ msg.common.close }}</button>
    </div>

    <div v-else-if="initialized" class="home-container">
        <div class="home-header">
            <h1 class="home-title">{{ msg.home.title }}</h1>
            <span class="home-badge initialized">{{ msg.home.badgeInitialized }}</span>
        </div>
        <p class="home-subtitle">{{ msg.home.subtitleInitialized }}</p>
        <div class="command-list">
            <CommandRow :command="c.enroll" clickable id="enroll-btn" @click="startEnroll"/>
            <CommandRow :command="c.link" clickable id="link-btn" @click="emit('navigate', {name: 'link', noFetch: false})"/>
            <CommandRow :command="c.linkNoFetch" clickable id="link-no-fetch-btn" @click="emit('navigate', {name: 'link', noFetch: true})"/>
            <CommandRow :command="c.promote" clickable id="promote-btn" @click="emit('navigate', {name: 'promote'})"/>
            <CommandRow :command="c.status" clickable id="status-btn" @click="emit('navigate', {name: 'status'})"/>
            <CommandRow :command="c.daemon"/>
            <CommandRow :command="c.diff"/>
            <CommandRow :command="c.config" clickable id="config-btn" @click="emit('navigate', {name: 'config'})"/>
            <CommandRow :command="c.daemonStatus" clickable id="daemon-status-btn" @click="emit('navigate', {name: 'daemonStatus'})"/>
            <CommandRow :command="c.daemonManagement" clickable id="daemon-management-btn" @click="emit('navigate', {name: 'daemonManagement'})"/>
            <CommandRow :command="c.reportIssue" clickable id="report-issue-btn" @click="emit('navigate', {name: 'reportIssue'})"/>
        </div>
        <button class="close-button" id="close-btn" @click="CloseWindow()">{{ msg.common.close }}</button>
    </div>

    <div v-else-if="initialized === false" class="home-container">
        <div class="home-header">
            <h1 class="home-title">{{ msg.home.title }}</h1>
            <span class="home-badge not-initialized">{{ msg.home.badgeNotInitialized }}</span>
        </div>
        <p class="home-subtitle">{{ msg.home.subtitleNotInitialized }}</p>
        <div class="steps">
            <div v-for="(step, i) in msg.home.steps" :key="i" class="step">
                <span class="step-number">{{ i + 1 }}</span>
                <div class="step-body">
                    <div class="step-label">{{ step.label }}</div>
                    <code class="step-cmd">{{ step.command }}</code>
                    <div class="step-hint">{{ step.hint }}</div>
                </div>
            </div>
        </div>
        <button class="control-btn" id="get-started-btn" @click="emit('navigate', {name: 'init'})">{{ msg.home.getStarted }}</button>
        <button class="close-button" id="close-btn" @click="CloseWindow()">{{ msg.common.close }}</button>
    </div>
</template>
