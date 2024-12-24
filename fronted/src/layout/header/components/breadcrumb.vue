<template>
    <div>
        <a-breadcrumb>
            <a-breadcrumb-item href="">
                <home-outlined />
            </a-breadcrumb-item>
            <a-breadcrumb-item href="" v-for="item in breadcrumbList">
                <!-- <user-outlined /> -->
                <span>{{ item.name }}</span>
            </a-breadcrumb-item>
        </a-breadcrumb>
    </div>
</template>

<script setup>

import {ref} from 'vue';
import {useRoute} from 'vue-router';
import {watch} from 'vue';
import {HomeOutlined} from '@ant-design/icons-vue';
const route = useRoute();
const breadcrumbList = ref([])
function setBreadcrumbList() {
    breadcrumbList.value = route.matched;
}
setBreadcrumbList();
watch(route,(New,Old) => {
    setBreadcrumbList();
})
// console.log(breadcrumbList)
</script>

<style scoped></style>