<script setup lang="ts">
import {computed} from 'vue';

const props = defineProps<{content: string}>();

function lineClass(line: string): string {
    if (line.startsWith('diff ') || line.startsWith('index ') || line.startsWith('---') || line.startsWith('+++')) {
        return 'diff-line diff-header';
    }
    if (line.startsWith('@@')) return 'diff-line diff-hunk';
    if (line.startsWith('+')) return 'diff-line diff-addition';
    if (line.startsWith('-')) return 'diff-line diff-deletion';
    return 'diff-line';
}

const lines = computed(() => {
    const c = props.content;
    return (c.endsWith('\n') ? c.slice(0, -1) : c).split('\n');
});
</script>

<template>
    <div v-for="(line, i) in lines" :key="i" :class="lineClass(line)">{{ line || ' ' }}</div>
</template>
