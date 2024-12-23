import { createStore } from 'vuex'
import { ref } from 'vue'

export default createStore({
    state: {
        activeKey: '/index',
        tabs: [
            {
                title: '首页',
                key: '/index'
            }

        ]
    },
    getters: {},
    mutations: {
        add_tab: (state, tab) => {
            //如果点击的菜单，已经是active的tab,则直接返回
            //查看是否已经存在
            let flag = state.tabs.findIndex(e => {
                return e.key == tab.key;
            })
            if (flag === -1) {
                //不存在，则添加
                state.tabs.push(
                    {
                        title: tab.title,
                        key: tab.key
                    }
                );
            }
            state.activeKey = tab.key;

        },
        remove_tab: (state, targetKey) => {
            
                let lastIndex = 0;
                state.tabs.forEach((tab, i) => {
                    if (tab.key === targetKey) {
                        // 表示删除的那个tab的前面一个的tab的key的index
                        lastIndex = i - 1;
                    }
                });
                //   删除被删除的key
                state.tabs = state.tabs.filter(tab => tab.key !== targetKey);
                console.log(state.tabs)
                //如果被删除的那个正好和活动的tab是同一个，且lastindex存在，则选择被删的的tab的前一个tab
                if (state.tabs.length && state.activeKey === targetKey) {
                    if (lastIndex >= 0) {
                        state.activeKey = state.tabs[lastIndex].key;
                    } else {
                        //否则选择第一个作为活动tab
                        state.activeKey = state.tabs[0].key;
                    }
                }
           
            
        },
        reset_tab: (state) => {
            state.activeKey = '/index';
            state.tabs = [
                {
                    title: '首页',
                    key: '/index'
                }
            ];
            
        }

    },
    actions: {},
    modules: {}
})



