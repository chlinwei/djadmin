export default createStore({
    state: {
        component_exclude: [],
        getters: {},
        mutations: {
            cache_route: (state, route) => {
                if(state.route_cache_exclude.has(route.path)) {
                    //不存在，则从缓存里取
                    route = route_cache_exclude.get(path)
                }else {
                    route = {
                        name: path,
                        render(){
                          return h(component) // h的第一个参数可以是字符串，也可以是一个组件定义；h返回的是一个虚拟dom
                        }
                      }
                      state.route_cache_exclude.set(path, route)
                    
                }
            }
        }
    },
})