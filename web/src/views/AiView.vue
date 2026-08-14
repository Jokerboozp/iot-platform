<script setup>
import { nextTick, ref } from 'vue'
import { api } from '../api'
const messages=ref([{role:'assistant',text:'你好，我可以协助查询设备、告警、趋势和消防知识。AI 不会直接控制设备或修改规则。'}]),question=ref(''),sending=ref(false),log=ref()
async function send(){const text=question.value.trim();if(!text||sending.value)return;messages.value.push({role:'user',text});question.value='';sending.value=true;try{const d=await api('/api/v1/ai/chat',{method:'POST',body:JSON.stringify({question:text})});messages.value.push({role:'assistant',text:d.answer})}catch(e){messages.value.push({role:'assistant',text:e.message})}finally{sending.value=false;await nextTick();log.value?.scrollTo({top:log.value.scrollHeight,behavior:'smooth'})}}
</script>
<template><el-card shadow="never" class="surface-card chat-card"><div ref="log" class="chat-log"><div v-for="(msg,i) in messages" :key="i" class="chat-message" :class="msg.role">{{msg.text}}</div></div><div class="chat-compose"><el-input v-model="question" placeholder="输入运维问题…" size="large" @keyup.enter="send" /><el-button type="primary" size="large" :loading="sending" @click="send">发送</el-button></div></el-card></template>
