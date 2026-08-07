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

La prueba final repetirá exactamente el mismo build y comando. El criterio original exigía además una prueba de humo en un teléfono físico, porque la emulación no la sustituye. El 7 de agosto de 2026 el responsable del proyecto ordenó omitirla para cerrar esta fase; por tanto, no existe evidencia de dispositivo real y el cierre conserva esa excepción de forma explícita.

## Resultado tras la implementación

La medición final repitió el mismo build, viewport, densidad, ralentización de CPU y comando que la línea de partida.

| Métrica | Resultado final | Límite | Estado |
|---|---:|---:|---:|
| JavaScript inicial minificado | 545,97 kB | 550 kB | Cumple |
| JavaScript inicial gzip | 168,09 kB | 170 kB | Cumple |
| CSS inicial minificado | 41,80 kB | 42 kB | Cumple |
| CSS inicial gzip | 7,71 kB | 8 kB | Cumple |
| Chunk diferido de preparación de fotos (JS gzip) | 1,77 kB | 25 kB | Cumple |
| Transferencia inicial observada | 590.851 bytes | 610.000 bytes | Cumple |
| First Contentful Paint | 1.396 ms | 1.500 ms | Cumple |
| FPS medio | 59,51 | 58 mínimo | Cumple |
| p95 por frame | 16,80 ms | 20 ms | Cumple |
| Frame máximo | 33,37 ms | 50 ms | Cumple |

El editor de fotos también genera un chunk CSS diferido de 1,75 kB minificados (0,70 kB gzip), que no se descarga hasta abrir el recorte. La solución 3D no incorpora WebGL ni dependencias nuevas: la profundidad y la inclinación usan perspectiva y transformaciones de Framer Motion/CSS ya presentes. Con movimiento reducido, estas transformaciones no se aplican.
