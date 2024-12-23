<template>
  <User />
  <Role />

  main 
  <nav>
<router-link to="/login2">主页</router-link>
<router-link to="/test">关于</router-link>
</nav>
  <router-view></router-view>
</template>
<script setup>
import { ref } from 'vue';
import User  from '@/views/sys/user/index.vue'
import Role  from '@/views/sys/role/index.vue'
</script>
<style scoped>
#components-layout-demo-side .logo {
  height: 32px;
  margin: 16px;
  background: rgba(255, 255, 255, 0.3);
}

.site-layout .site-layout-background {
  background: #fff;
}
[data-theme='dark'] .site-layout .site-layout-background {
  background: #141414;
}
</style>