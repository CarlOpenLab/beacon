import './style.css'
import { App } from './app'

const container = document.getElementById('app')
if (container) {
  const app = new App(container)
  app.start()
}