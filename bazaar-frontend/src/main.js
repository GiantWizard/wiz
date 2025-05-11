// src/main.js
import './app.css'; // Optional global CSS
import App from './App.svelte'; // This is your ROUTER component
import { mount } from 'svelte';

const app = mount(App, {
  target: document.getElementById('app'),
  // props: {} // if your App.svelte (router) accepted props
});

export default app;