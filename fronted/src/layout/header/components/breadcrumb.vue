<template>
    <div class="breadcrumb">
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
    console.log("fds")
    console.log(route.matched)
}
setBreadcrumbList();
watch(route,(New,Old) => {
    setBreadcrumbList();
})
</script>

<style scoped>
.breadcrumb {
    display: flex;
    align-items: center;
    height: 100%;
}
</style>