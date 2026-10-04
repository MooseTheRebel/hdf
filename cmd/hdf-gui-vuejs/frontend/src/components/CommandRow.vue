<script setup lang="ts">
// CommandRow is a home-screen command: a button when clickable, otherwise
// a plain row.
defineProps<{command: {command: string; description: string}; clickable?: boolean}>();
</script>

<template>
    <component :is="clickable ? 'button' : 'div'" class="command-row" :class="{'command-row-clickable': clickable}">
        <code class="cmd">{{ command.command }}</code>
        <span class="cmd-desc">{{ command.description }}</span>
    </component>
</template>
