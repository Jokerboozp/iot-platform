<script setup>
import { cn } from '../../lib/utils'

defineProps({
  class: { type: String, default: '' }
})
</script>

<template>
  <div :class="cn('relative flex size-9 shrink-0 items-center justify-center overflow-hidden rounded-full bg-primary text-sm font-semibold text-primary-foreground', $props.class)">
    <slot />
  </div>
</template>
