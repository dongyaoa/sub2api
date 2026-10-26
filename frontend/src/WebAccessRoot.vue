<script setup lang="ts">
import App from './App.vue'
import WebAccessPage from '@/components/common/WebAccessPage.vue'
import { webAccessGate } from '@/services/webAccess'
</script>

<template>
  <App v-if="!webAccessGate.state.enabled || webAccessGate.state.phase === 'allowed'" />
  <WebAccessPage v-else :phase="webAccessGate.state.phase" :message="webAccessGate.state.message" :checking="webAccessGate.state.checking" />
</template>
