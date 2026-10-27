import './styles/tokens.css';
import './styles/base.css';
import { mount } from 'svelte';
import App from './App.svelte';
import './widgets'; // registers all widget types

mount(App, { target: document.getElementById('app')! });
