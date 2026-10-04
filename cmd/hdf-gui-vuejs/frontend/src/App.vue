<script setup lang="ts">
import {ref} from 'vue';
import {HasDiff} from '../wailsjs/go/cli/App';
import type {View} from './view';
import HomeView from './views/HomeView.vue';
import DiffViewer from './views/DiffViewer.vue';
import StatusView from './views/StatusView.vue';
import ConfigView from './views/ConfigView.vue';
import DaemonStatusView from './views/DaemonStatusView.vue';
import DaemonManagementView from './views/DaemonManagementView.vue';
import InitView from './views/InitView.vue';
import EnrollView from './views/EnrollView.vue';
import LinkView from './views/LinkView.vue';
import PromoteView from './views/PromoteView.vue';
import ReportIssueView from './views/ReportIssueView.vue';

const view = ref<View | null>(null);

HasDiff()
    .then((hasDiff) => { view.value = hasDiff ? {name: 'diff'} : {name: 'home'}; })
    .catch(() => { view.value = {name: 'home'}; });

function go(next: View) {
    view.value = next;
}

function home() {
    go({name: 'home'});
}
</script>

<template>
    <template v-if="view">
        <HomeView v-if="view.name === 'home'" @navigate="go"/>
        <DiffViewer v-else-if="view.name === 'diff'"/>
        <StatusView v-else-if="view.name === 'status'" @back="home"/>
        <ConfigView v-else-if="view.name === 'config'" @back="home"/>
        <DaemonStatusView v-else-if="view.name === 'daemonStatus'" @back="home"/>
        <DaemonManagementView v-else-if="view.name === 'daemonManagement'" @back="home"/>
        <InitView v-else-if="view.name === 'init'" @back="home"/>
        <EnrollView v-else-if="view.name === 'enroll'" :path="view.path" :pick-error="view.pickError" @back="home"/>
        <LinkView v-else-if="view.name === 'link'" :no-fetch="view.noFetch" @back="home"/>
        <PromoteView v-else-if="view.name === 'promote'" @back="home"/>
        <ReportIssueView v-else-if="view.name === 'reportIssue'" @back="home"/>
    </template>
</template>
