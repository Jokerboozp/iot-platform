<script setup>
import { computed } from 'vue'
import { cva } from 'class-variance-authority'
import { cn } from '../../lib/utils'

const badgeVariants = cva('inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring/50', {
  variants: {
    variant: {
      default: 'border-transparent bg-primary text-primary-foreground',
      secondary: 'border-transparent bg-secondary text-secondary-foreground',
      outline: 'text-foreground',
      success: 'border-transparent bg-success/15 text-success-foreground',
      warning: 'border-transparent bg-warning/15 text-warning-foreground',
      destructive: 'border-transparent bg-destructive/15 text-destructive-foreground'
    }
  },
  defaultVariants: { variant: 'default' }
})

const props = defineProps({
  variant: { type: String, default: 'default' },
  class: { type: String, default: '' }
})

const classes = computed(() => cn(badgeVariants({ variant: props.variant }), props.class))
</script>

<template><span :class="classes"><slot /></span></template>
