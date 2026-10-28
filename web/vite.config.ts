import { defineConfig } from 'vite';
export default defineConfig({server:{proxy:{'/api':'http://localhost:8080','/healthz':'http://localhost:8080','/readyz':'http://localhost:8080'}}});
