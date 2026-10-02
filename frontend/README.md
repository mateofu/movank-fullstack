# Frontend

SvelteKit y TypeScript: catálogo, carrito, pago, resultado y dashboard en vivo.
El carrito y la operación en curso se guardan en IndexedDB por comercio y usuario.
Todavía no hay instalación PWA ni sincronización offline.

## Ejecutar

Con el backend y las migraciones preparados, desde la raíz:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File infra/demo.ps1
cd frontend
npm.cmd ci
npm.cmd run dev
```

Abre http://127.0.0.1:5173 y pega el token que imprimió el script. El script crea
un comercio de ejemplo sin sobrescribir productos existentes. El token dura 15 minutos;
puedes ejecutar el script de nuevo para renovarlo.

Si cambiaste el puerto del backend, configura `API_URL` antes de arrancar Vite.
La sesión usa cookie HttpOnly; el token no se guarda en IndexedDB ni localStorage.
Una venta interrumpida conserva su clave y escenario para reintentar sin duplicarla.

## Verificar

Desde `frontend`, con Node 24 y el backend activo:

```powershell
npm.cmd run check
npm.cmd test
npm.cmd run build
npm.cmd run test:e2e
```

Las pruebas de navegador usan Edge instalado y eliminan sus comercios de prueba.
Cubren recarga, aislamiento, dos pestañas, respuesta perdida, pagos y precios cambiados.
`build/` contiene archivos estáticos; `npm.cmd run preview` permite probarlos localmente.
