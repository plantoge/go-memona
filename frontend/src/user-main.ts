import { mount } from 'svelte';
import UserApp from './UserApp.svelte';
import './style.css';
import './user-dashboard.css';
import { startTheme } from './lib/theme';

startTheme();
mount(UserApp, { target: document.getElementById('app')! });
