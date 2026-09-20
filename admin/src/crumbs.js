// 面包屑由当前视图填，外壳只负责画。除了最后一节都可以点回去——
// 钻进三层之后没有返回路径是很烦的。
import { ref } from 'vue'
export const crumbs = ref([])
export const setCrumbs = (list) => { crumbs.value = list }
