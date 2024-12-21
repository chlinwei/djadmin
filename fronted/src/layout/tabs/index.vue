<template>
    <div>
        fff
        <a-breadcrumb style="margin: 16px 0">
            <a-breadcrumb-item>User</a-breadcrumb-item>
            <a-breadcrumb-item>Bill</a-breadcrumb-item>
        </a-breadcrumb>
        <div :style="{ padding: '24px', background: '#fff', minHeight: '360px' }">
            Bill is a cat.
        </div>
    </div>
</template>
<script>
import { defineComponent, ref } from 'vue';
import requestUtil from '@/util/request';

// export default defineComponent({
//     data() {
//         return {
//             collapsed: ref(false),
//             selectedKeys: ref(['1']),
//         };
//     },
// });
</script>
<style>

</style>