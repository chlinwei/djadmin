<template>
    <div>
      <a-modal v-model:open="props.open" v-model:title="props.title" @ok="handleOk">
        <p>Some contents...</p>
        <p>Some contents...</p>
        <p>Some contents...</p>
      </a-modal>
    </div>
  </template>
  <script setup>
  import { ref } from 'vue';
  const props = defineProps(
    {
        open: {
            type: Boolean,
            default:false,
            required: true
        },
        title: {
            type: String,
            default: '错误界面',
            required: true
        },
        user_id: {
            type: Number,
            default: -1,
            required: true
        },
    }
  )
  const emits = defineEmits(['update:open'])
  const open = ref(false);
  const title = ref("错误界面1")
  const showModal = () => {
    open.value = true;
  };
  const handleOk = e => {
    emits('update:open',false);
  };
  </script>