<template>
    <a-modal cancelText="取消" okText="保存" destroyOnClose :open="props.open2"  v-model:user_id2="props.user_id2" @ok="handleOk" @cancel="handleCancel">
        <div>
    <a-transfer
      v-model:target-keys="targetKeys"
      v-model:selected-keys="selectedKeys"
      :data-source="mockData"
      :titles="['所有角色', '当前用户']"
      :render="item => item.title"
      :disabled="disabled"
      @change="handleChange"
      @selectChange="handleSelectChange"
      @scroll="handleScroll"
    />
    <a-switch
      v-model:checked="disabled"
      un-checked-children="enabled"
      checked-children="disabled"
      style="margin-top: 16px"
    />
  </div>
    </a-modal>

    
</template>

<script setup>
import { ref } from 'vue';
  const props = defineProps(
    {
        open2: {
            type: Boolean,
            default:false,
            required: true
        },
        user_id2: {
            type: Number,
            default: -1,
            required: true
        },
    }
  )

  const emits = defineEmits(['update:open2','initUserList'])
  const handleCancel = ()=>{
    emits('update:open2',false)
  }
  const handleOk = ()=> {
    console.log("ok")
  }



  const mockData = [];
for (let i = 0; i < 20; i++) {
  mockData.push({
    key: i.toString(),
    title: `content${i + 1}`,
    description: `description of content${i + 1}`,
    disabled: i % 3 < 1,
  });
}
const oriTargetKeys = mockData.filter(item => +item.key % 3 > 1).map(item => item.key);
const disabled = ref(false);
const targetKeys = ref(oriTargetKeys);
const selectedKeys = ref(['1', '4']);
const handleChange = (nextTargetKeys, direction, moveKeys) => {
  console.log('targetKeys: ', nextTargetKeys);
  console.log('direction: ', direction);
  console.log('moveKeys: ', moveKeys);
};
const handleSelectChange = (sourceSelectedKeys, targetSelectedKeys) => {
  console.log('sourceSelectedKeys: ', sourceSelectedKeys);
  console.log('targetSelectedKeys: ', targetSelectedKeys);
};
const handleScroll = (direction, e) => {
  console.log('direction:', direction);
  console.log('target:', e.target);
};
</script>
<style scoped>
</style>