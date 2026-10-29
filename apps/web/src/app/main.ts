import { createApp } from "vue"
import { router } from "./router"
import { pinia } from "./pinia"
import App from "./ui/App.vue"
import "./styles.css"

/**
 * The app's entry point: one mount, one router, one store container.
 *
 * Nothing is configured before this runs. The API base is the origin that served
 * this page (`shared/api` defaults to a relative base), which is the same origin
 * in production and Vite's proxied origin in development — there is no runtime
 * setting to resolve, so there is no settings load to await.
 *
 * `./styles.css` is imported first, for its side effect only: it is the Tailwind
 * entry and the bridge that aliases shadcn's tokens onto the palette
 * `index.html` owns. Without it the `shared/ui` primitives would render
 * unstyled rather than wrongly styled, which is the failure mode worth having.
 */
const app = createApp(App)

app.use(pinia)
app.use(router)
app.mount("#app")
