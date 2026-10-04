<script setup lang="ts">
// StepPage is the layout shared by the multi-step flows (init, enroll,
// link, promote, report issue): a title, the current step's content, and
// its buttons.
// contentId exists only because the shared CSS styles the link flow's
// content by ID (#link-step-content).
defineProps<{title: string; contentId?: string}>();
</script>

<template>
    <div class="link-container">
        <div class="link-header-section">
            <h1>{{ title }}</h1>
        </div>
        <div class="step-content" :id="contentId"><slot/></div>
        <div class="link-controls"><slot name="controls"/></div>
    </div>
</template>
