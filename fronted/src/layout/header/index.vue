<template>
    <!-- <div>

    </div> -->

    <a-row>
        <a-col :span="16">
            <BreadCrumb />
        </a-col>
        <a-col :span="8">
            <Avatar />
        </a-col>
        
       
  </a-row>
</template>
<script setup>
import Avatar from '@/layout/header/components/avatar.vue'
import BreadCrumb from '@/layout/header/components/breadcrumb.vue'
</script>
<style scoped></style>