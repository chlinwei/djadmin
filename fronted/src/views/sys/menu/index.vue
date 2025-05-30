<template>
  <a-tree
default-expand-all
   
   
    checkable
    :tree-data="treeData"
  >
    <template #title="{ title, key }">
      <span v-if="key === '0-0-1-0'" style="color: #1890ff">{{ title }}</span>
      <template v-else>{{ title }}</template>
    </template>
  </a-tree>
</template>
<script setup>
import { ref, watch } from 'vue';
const treeData = [
    {
        title: 'parebnt2',
        key:'哈哈哈',
    },

  {
    title: 'parent 1',
    key: '0-0',
    children: [
      {
        title: 'parent 1-0',
        key: '0-0-0-0ccc',
        disabled: true,
        children: [
          {
            title: 'leaf',
            key: '0-0-0-0ccc',
            disableCheckbox: true,
          },
          {
            title: 'leaf',
            key: '0-0fdsfds-0-fsdaf1',
          },
        ],
      },
      {
        title: 'parent 1-1',
        key: '0-0-1',
        children: [
          {
            key: '0c0',
            title: 'sss',
          },
        ],
      },
    ],
  },
];

const selectedKeys = ref(['0-0-0', '0-0-1']);
const checkedKeys = ref(['0-0-0', '0-0-1']);
// watch(expandedKeys, () => {
//   console.log('expandedKeys', expandedKeys);
// });
// watch(selectedKeys, () => {
//   console.log('selectedKeys', selectedKeys);
// });
// watch(checkedKeys, () => {
//   console.log('checkedKeys', checkedKeys);
// });
</script>