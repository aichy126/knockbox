import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { i18n } from './i18n'
import './style.css'

// 等路由解析完再挂载。不等的话首帧的 route.name 还是 undefined，App.vue 会先
// 画出一个空外壳（顶栏「?」、版本号空），下一帧才切到真正的页面——
// 未登录的人第一眼看到的就是那个空后台。
const app = createApp(App).use(router).use(i18n)
router.isReady().then(() => app.mount('#app'))
