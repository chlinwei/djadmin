<template>
    <a-tree
      v-model:expandedKeys="expandedKeys"
      v-model:selectedKeys="selectedKeys"
      v-model:checkedKeys="checkedKeys"
      checkable
      checkStrictly
      :tree-data="treeData"
      @select="handleSelect"
    >

    </a-tree>
  </template>
  <script setup>
  import { ref, watch } from 'vue';
  const treeData = [
    {
      title: '0-0',
      key: '0-0',
      children: [
        {
          title: '0-0-0',
          key: '0-0-0',
    
          children: [
            {
              title: '0-0-0-0',
              key: '0-0-0-0',
            
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
          ],
        },
      ],
    },
  ];
  const expandedKeys = ref(['0-0-0', '0-0-1']);
//  const selectedKeys = ref([]);
  const selectedKeys = ref(['0-0-0-0', '0-0-1-0']);
//   const checkedKeys = ref(['0-0-0', '0-0-1']);
const checkedKeys = ref(['0-0-0-0', '0-0-1-0']);
  watch(expandedKeys, () => {
    console.log('expandedKeys', expandedKeys);
  });
  watch(selectedKeys, () => {
    console.log('selectedKeys', selectedKeys);
  });
  watch(checkedKeys, () => {
    console.log('checkedKeys', checkedKeys);
    console.log("selected_keys",selectedKeys)
  });
  const handleSelect = (res)=> {
    console.log("fffffffffff")
    console.log('select', res);
 
  }
  const check = (res) =>{
    console.log('check', res);
  }
  console.log(selectedKeys)
  </script>