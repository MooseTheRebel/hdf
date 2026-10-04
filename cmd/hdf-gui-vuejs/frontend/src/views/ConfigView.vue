<script setup lang="ts">
import {ref} from 'vue';
import {fmt, msg} from '@locales';
import {GetConfig} from '../../wailsjs/go/cli/App';
import type {cli} from '../../wailsjs/go/models';
import I18nText from '../components/I18nText';

const emit = defineEmits<{back: []}>();

const info = ref<cli.ConfigInfo | null>(null);
const error = ref('');

GetConfig()
    .then((v) => { info.value = v; })
    .catch((err) => { error.value = fmt(msg.config.loadError, {error: String(err)}); });
</script>

<template>
    <div class="config-container">
        <div class="config-header-section">
            <h1>{{ msg.config.title }}</h1>
        </div>
        <div v-if="error">{{ error }}</div>
        <div v-else-if="!info">{{ msg.config.loading }}</div>
        <p v-else-if="!info.exists" class="config-missing">
            <I18nText :template="msg.config.missing"><template #command><code>{{ msg.config.missingCommand }}</code></template></I18nText>
        </p>
        <div v-else class="config-details">
            <div class="config-field"><span class="config-label">{{ msg.config.file }}</span><span class="config-value">{{ info.path }}</span></div>
            <pre class="config-content">{{ info.content }}</pre>
        </div>
        <div class="config-controls">
            <button class="control-btn" @click="emit('back')">{{ msg.common.back }}</button>
        </div>
    </div>
</template>
