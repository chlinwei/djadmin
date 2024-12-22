import {createStore} from 'vuex'
import {ref} from 'vue'

export default createStore({
    state: {
        activeKey: '/index',
        tabs:[
            {
                title: '首页',
                key: '/index'
            }

        ]
    },
    getters: {},
    mutations: {
        add_tab: (state,tab) => {
            //如果点击的菜单，已经是active的tab,则直接返回
            //查看是否已经存在
            let flag = state.tabs.findIndex(e => {
                return e.key == tab.key;
            })
            if(flag === -1) {
                //不存在，则添加
                state.tabs.push(
                    {
                        title: tab.title,
                        key: state.activeKey
                    }
                );
            }
            state.activeKey = tab.key;
            console.log(state.tabs)

        },

    },
    actions: {},
    modules: {}
})



 