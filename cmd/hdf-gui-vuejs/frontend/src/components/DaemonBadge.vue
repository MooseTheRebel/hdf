<script setup lang="ts">
import {computed} from 'vue';

const props = defineProps<{status: string}>();

const modifier = computed(() => {
    switch (props.status) {
        case 'running':
            return 'daemon-running';
        case 'stopped':
            return 'daemon-stopped';
        case 'not installed':
            return 'daemon-not-installed';
        default:
            return 'daemon-unknown';
    }
});
</script>

<template>
    <span class="home-badge" :class="modifier">{{ status }}</span>
</template>
