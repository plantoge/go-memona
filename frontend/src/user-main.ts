import { mount } from 'svelte';
import UserApp from './UserApp.svelte';
import './user-style.css';

mount(UserApp, { target: document.getElementById('app')! });
