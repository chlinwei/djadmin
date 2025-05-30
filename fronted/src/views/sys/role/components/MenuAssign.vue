<template>
  <a-tree
default-expand-all
    checkable
    :tree-data="treeData"
  >
    <span>你好</span>
  </a-tree>
</template>
<script setup>
import { ref, watch } from 'vue';
import {getMenuTree} from '@/api/menu';
const treeData = ref([])

const selectedKeys = ref([]);
const checkedKeys = ref([]);

// 解析数据为树形结构
const parseTreeData = (data) => {
  return data.map(item => ({
    title: item.name,
    key: item.id,
    // icon: <a-icon type={item.icon} />,  // 注意：这里使用了Ant Design的图标组件，需要根据实际情况调整
    // disabled: !item.perms || item.perms.length === 0,  // 根据perms字段判断是否有权限
    children: item.children ? parseTreeData(item.children) : [],
  }));
};

getMenuTree().then((res)=>{
  var data = parseTreeData(res.data.data)
  treeData.value = data
  console.log(data)
})
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