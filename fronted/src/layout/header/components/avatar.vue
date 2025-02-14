<template>
    <router-link :to="{ name: '个人中心' }">个人中心</router-link>
</template>

<script setup>
import {useRouter} from 'vue-router'
console.log("avatar:")
console.log(useRouter().currentRoute.value.fullPath)

</script>
<style scoped></style> 