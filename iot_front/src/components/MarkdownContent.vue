<script setup>
import { computed } from 'vue'
import { renderMarkdown } from '../markdown'

const props = defineProps({
  source: { type: [String, Number], default: '' }
})

const html = computed(() => renderMarkdown(props.source))
</script>

<template>
  <div class="markdown-content" v-html="html"></div>
</template>
