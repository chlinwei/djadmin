<template>
  <RouterView />
</template>
<script setup>
import {  RouterView, useRoute } from 'vue-router';
import {watch} from 'vue';
import router from '@/router';
const route = useRoute();
import store from '@/store'
watch(route,(to,from,next) => {
    if(to.name == '个人中心') {
        let obj = {
            title: to.name,
            key: to.path
        }
        store.commit('add_tab',obj)
        store.commit('set_selectedKeys',[])
        let all_routes = router.getRoutes()
     all_routes.forEach(e => {
      if(e.path == to.path) {
        e.meta.cached = true;
        console.log("=======hahahh=========")
        console.log(e)
      }
     })

    }
    
}, {deep: true, immediate: true})
import {useRouter} from 'vue-router';
console.log("====app====")
console.log(useRouter().currentRoute.value)
</script>

<style>
html,body,#app {
  height: 100%;
}
</style>
