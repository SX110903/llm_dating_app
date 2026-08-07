import { mkdtemp, readFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { spawn } from "node:child_process"

class CDPClient {
  static async connect(url) {
    const socket = new WebSocket(url)
    await new Promise((resolve, reject) => {
      socket.addEventListener("open", resolve, { once: true })
      socket.addEventListener("error", reject, { once: true })
    })
    return new CDPClient(socket)
  }

  constructor(socket) {
    this.socket = socket
    this.nextID = 1
    this.pending = new Map()
    this.listeners = new Map()
    socket.addEventListener("message", (event) => this.handleMessage(JSON.parse(event.data)))
  }

  send(method, params = {}) {
    const id = this.nextID
    this.nextID += 1
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject })
      this.socket.send(JSON.stringify({ id, method, params }))
    })
  }

  on(method, listener) {
    const listeners = this.listeners.get(method) ?? []
    listeners.push(listener)
    this.listeners.set(method, listeners)
  }

  once(method) {
    return new Promise((resolve) => {
      const listener = (params) => {
        const listeners = this.listeners.get(method) ?? []
        this.listeners.set(method, listeners.filter((candidate) => candidate !== listener))
        resolve(params)
      }
      this.on(method, listener)
    })
  }

  close() {
    this.socket.close()
  }

  handleMessage(message) {
    if (message.id !== undefined) {
      const pending = this.pending.get(message.id)
      if (!pending) return
      this.pending.delete(message.id)
      if (message.error) pending.reject(new Error(message.error.message))
      else pending.resolve(message.result)
      return
    }
    for (const listener of this.listeners.get(message.method) ?? []) listener(message.params)
  }
}

const targetURL = process.argv[2] ?? "http://localhost:8080/"
const chromePath = process.env.LLMATCH_CHROME_PATH ?? "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
const profileDirectory = await mkdtemp(join(tmpdir(), "llmatch-performance-"))
const chrome = spawn(
  chromePath,
  [
    "--headless=new",
    "--disable-gpu",
    "--no-first-run",
    "--no-default-browser-check",
    "--remote-debugging-port=0",
    `--user-data-dir=${profileDirectory}`,
    "about:blank",
  ],
  { stdio: "ignore", windowsHide: true },
)

try {
  const port = await readDebuggingPort(profileDirectory)
  const target = await createTarget(port, targetURL)
  const client = await CDPClient.connect(target.webSocketDebuggerUrl)

  try {
    let transferredBytes = 0
    client.on("Network.loadingFinished", (params) => {
      transferredBytes += params.encodedDataLength ?? 0
    })

    await Promise.all([
      client.send("Page.enable"),
      client.send("Network.enable"),
      client.send("Performance.enable"),
      client.send("Runtime.enable"),
    ])
    await client.send("Emulation.setDeviceMetricsOverride", {
      width: 375,
      height: 812,
      deviceScaleFactor: 2,
      mobile: true,
    })
    await client.send("Emulation.setCPUThrottlingRate", { rate: 4 })
    await client.send("Page.addScriptToEvaluateOnNewDocument", {
      source: `
        window.__llmatchLongTasks = [];
        new PerformanceObserver((list) => {
          window.__llmatchLongTasks.push(...list.getEntries().map((entry) => entry.duration));
        }).observe({ type: "longtask", buffered: true });
      `,
    })

    const loaded = client.once("Page.loadEventFired")
    await client.send("Page.navigate", { url: targetURL })
    await loaded

    const frameResult = await client.send("Runtime.evaluate", {
      expression: `new Promise((resolve) => {
        const deltas = [];
        let started;
        let previous;
        function frame(now) {
          if (started === undefined) {
            started = now;
            previous = now;
          } else {
            deltas.push(now - previous);
            previous = now;
          }
          if (now - started >= 2000) {
            const ordered = [...deltas].sort((a, b) => a - b);
            const mean = deltas.reduce((total, value) => total + value, 0) / deltas.length;
            resolve({
              frames: deltas.length,
              fps: 1000 / mean,
              meanFrameMs: mean,
              p95FrameMs: ordered[Math.floor(ordered.length * 0.95)] ?? 0,
              maxFrameMs: ordered.at(-1) ?? 0,
            });
            return;
          }
          requestAnimationFrame(frame);
        }
        requestAnimationFrame(frame);
      })`,
      awaitPromise: true,
      returnByValue: true,
    })
    const timingResult = await client.send("Runtime.evaluate", {
      expression: `(() => {
        const navigation = performance.getEntriesByType("navigation")[0];
        const paints = Object.fromEntries(performance.getEntriesByType("paint").map((entry) => [entry.name, entry.startTime]));
        const longTasks = window.__llmatchLongTasks ?? [];
        return {
          domContentLoadedMs: navigation?.domContentLoadedEventEnd ?? 0,
          loadMs: navigation?.loadEventEnd ?? 0,
          firstContentfulPaintMs: paints["first-contentful-paint"] ?? 0,
          longTaskCount: longTasks.length,
          longestTaskMs: longTasks.length ? Math.max(...longTasks) : 0,
        };
      })()`,
      returnByValue: true,
    })
    const performanceMetrics = await client.send("Performance.getMetrics")
    const metrics = Object.fromEntries(performanceMetrics.metrics.map(({ name, value }) => [name, value]))

    console.log(
      JSON.stringify(
        {
          url: targetURL,
          emulation: { viewport: "375x812", deviceScaleFactor: 2, cpuSlowdown: 4 },
          transferBytes: Math.round(transferredBytes),
          ...timingResult.result.value,
          ...frameResult.result.value,
          scriptDurationMs: Math.round((metrics.ScriptDuration ?? 0) * 1000),
          taskDurationMs: Math.round((metrics.TaskDuration ?? 0) * 1000),
          jsHeapUsedBytes: Math.round(metrics.JSHeapUsedSize ?? 0),
        },
        null,
        2,
      ),
    )
  } finally {
    client.close()
  }
} finally {
  if (chrome.exitCode === null) {
    const stopped = new Promise((resolve) => chrome.once("exit", resolve))
    chrome.kill()
    await Promise.race([stopped, new Promise((resolve) => setTimeout(resolve, 5000))])
  }
  await rm(profileDirectory, { force: true, recursive: true, maxRetries: 10, retryDelay: 100 })
}

async function readDebuggingPort(directory) {
  const path = join(directory, "DevToolsActivePort")
  for (let attempt = 0; attempt < 100; attempt += 1) {
    try {
      const [port] = (await readFile(path, "utf8")).trim().split(/\r?\n/)
      return Number(port)
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 50))
    }
  }
  throw new Error("Chrome did not expose a debugging port")
}

async function createTarget(port, url) {
  const response = await fetch(`http://127.0.0.1:${port}/json/new?${encodeURIComponent(url)}`, { method: "PUT" })
  if (!response.ok) throw new Error(`Could not create Chrome target: ${response.status}`)
  return response.json()
}
