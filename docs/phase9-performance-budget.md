# Fase 9 — presupuesto de rendimiento

Medición de partida registrada el 7 de agosto de 2026 sobre `8a03fcc`, antes de escribir código de animación de la Fase 9.

## Método reproducible

- Build de producción con Node 24.14.0 y Vite 8.2.0.
- Chrome en modo headless contra la imagen Nginx de producción local.
- Viewport 375×812, densidad 2× y CPU ralentizada 4× como aproximación conservadora a un móvil de gama media.
- Muestreo de `requestAnimationFrame` durante dos segundos después de la carga.
- Comando: `node scripts/measure-frontend-performance.mjs http://localhost:8081/`.

El medidor no aplica latencia de red artificial. Los tiempos de carga sirven para comparar el mismo entorno antes y después, no para representar una conexión móvil pública.

## Línea de partida

| Métrica | Valor inicial |
|---|---:|
| JavaScript inicial minificado | 539,89 kB |
| JavaScript inicial gzip | 166,02 kB |
| CSS inicial minificado | 39,48 kB |
| CSS inicial gzip | 7,31 kB |
| Transferencia total observada | 582.458 bytes |
| First Contentful Paint | 1.228 ms |
| DOM Content Loaded | 927 ms |
| FPS medio | 60,00 |
| Tiempo medio por frame | 16,67 ms |
| p95 por frame | 16,70 ms |
| Frame máximo | 16,80 ms |
| Tareas largas durante arranque | 3 |
| Tarea de arranque más larga | 280 ms |

## Límites de la fase

- Objetivo de animación: 60 fps. Criterio automatizado: al menos 58 fps de media, p95 de frame no superior a 20 ms y ningún frame superior a 50 ms con la emulación anterior.
- JavaScript inicial: máximo 550 kB minificados y 170 kB gzip. La funcionalidad de recorte/compresión se cargará de forma diferida y no contará en el camino inicial hasta que el usuario abra la subida.
- Chunk diferido de preparación de fotos: máximo 25 kB gzip.
- CSS inicial: máximo 42 kB minificados y 8 kB gzip.
- Transferencia inicial en la medición local: máximo 610.000 bytes.
- First Contentful Paint en el mismo entorno: máximo 1.500 ms.
- No se incorpora motor WebGL. La profundidad visual se resolverá con `transform`, `perspective` y composición acelerada por CSS.
- Toda animación nueva tendrá una variante sin movimiento cuando `prefers-reduced-motion` esté activo.

La prueba final repetirá exactamente el mismo build y comando. La fase no se cerrará sin una prueba de humo adicional en un teléfono físico; la emulación no la sustituye.
