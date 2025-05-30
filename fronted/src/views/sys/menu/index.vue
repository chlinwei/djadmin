<template>
    <a-tree
    default-expand-all
      v-model:selectedKeys="selectedKeys"
      v-model:checkedKeys="checkedKeys"
      checkable
      :tree-data="treeData"
    >

    </a-tree>
  </template>
  <script setup>
  import { ref, watch } from 'vue';
  const treeData = [
    {
        title: 'a',
        key: 'a',
        children:[
            {
                title: 'a-1',
                key: 'a-1',
            },
            {
                title: 'a-2',
                key: 'a-2',
            },
        ]
    },
    {
      title: '0-0',
      key: '0-0',
      children: [
        {
          title: '0-0-0',
          key: '0-0-0',
          disabled: false,
          children: [
            {
              title: '0-0-0-0',
              key: '0-0-0-0',
              disableCheckbox: false,
            },
            {
              title: '0-0-0-1',
              key: '0-0-0-1',
            },
          ],
        },
        {
          title: '0-0-1',
          key: '0-0-1',
          children: [
            {
              key: '0-0-1-0',
              title: '0-0-1-0',
            },
            {
              key: '0-0-1-2',
              title: '0-0-1-2',
            },
          ],
        },
      ],
    },
  ];
  const expandedKeys = ref(['0-0-0', '0-0-1']);
  const selectedKeys = ref(['0-0-0', '0-0-1']);
  const checkedKeys = ref(['0-0-0', '0-0-1']);
  watch(expandedKeys, () => {
    console.log('expandedKeys', expandedKeys);
  });
  watch(selectedKeys, () => {
    console.log('checkedKeys', checkedKeys);
    console.log('selectedKeys', selectedKeys);
  });
  watch(checkedKeys, () => {
    console.log('checkedKeys', checkedKeys);
    console.log('selectedKeys', selectedKeys);
  });
  </script>