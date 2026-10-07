const k = 1, S = "filex:hello", E = "filex:port", L = [
  "session.get",
  "file.read",
  "file.save",
  "file.saveAs",
  "ui.dirty",
  "ui.title",
  "ui.toast",
  "ui.confirm",
  "ui.close",
  "clipboard.write",
  "ui.download",
  "engine.call",
  "job.submit",
  "state.get",
  "state.set",
  "license.get"
], x = {
  /** `file.read({as: 'text'})` refuses a file larger than this; read a stream. */
  maxTextBytes: 33554432,
  /** One `state.set` value, as JSON. */
  maxStateBytes: 8192,
  /** A `ui.toast` / `ui.title` text, in characters. */
  maxLineChars: 300,
  /** How long the SDK waits for the host's port. */
  connectTimeoutMs: 1e4,
  /** The largest file `ui.download` hands the person (streamed or not). */
  maxDownloadBytes: 268435456
};
function I(n) {
  return !!n && typeof n == "object" && n.type === S && typeof n.v == "number";
}
function B(n) {
  return !!n && typeof n == "object" && n.type === E;
}
function j(n) {
  return !!n && typeof n == "object" && typeof n.id == "number" && typeof n.method == "string";
}
function q(n) {
  return L.includes(n);
}
class p extends Error {
  constructor(c) {
    super(c.message || c.code), this.name = "FilexError", this.code = c.code;
  }
}
let b = null;
function A(n = {}) {
  return b || (b = O(n)), b;
}
function O(n) {
  return new Promise((c, d) => {
    if (typeof window > "u" || window.parent === window) {
      d(new p({ code: "unavailable", message: "not running inside filex" }));
      return;
    }
    const i = window.parent, u = R(), a = setTimeout(() => {
      window.removeEventListener("message", g), d(new p({ code: "unavailable", message: "filex did not answer" }));
    }, n.timeoutMs ?? x.connectTimeoutMs);
    function g(y) {
      if (y.source !== i || !B(y.data) || !y.ports[0] || u && y.origin !== u) return;
      window.removeEventListener("message", g), clearTimeout(a);
      const r = y.ports[0], f = T(r, n);
      f.request("session.get").then((m) => {
        f.session = m, n.applyTheme !== !1 && (v(m.theme, m.locale, m.dir), f.on("theme", (l) => v(l)), f.on("locale", (l) => {
          const e = l;
          v(void 0, e.locale, e.dir);
        })), c(f);
      }).catch(d);
    }
    window.addEventListener("message", g), i.postMessage({ type: S, v: 1 }, "*");
  });
}
function R() {
  try {
    const n = window.location.ancestorOrigins;
    if (n && n.length > 0 && n[0] !== "null") return n[0];
  } catch {
  }
  return "";
}
function T(n, c) {
  let d = 0;
  const i = /* @__PURE__ */ new Map(), u = /* @__PURE__ */ new Map();
  let a = null;
  n.onmessage = (e) => {
    const t = e.data;
    if (!(!t || typeof t != "object")) {
      if (typeof t.id == "number") {
        const s = i.get(t.id);
        if (!s) return;
        i.delete(t.id), t.error ? s.reject(new p(t.error)) : s.resolve(t.result);
        return;
      }
      if (typeof t.hid == "number") {
        g(t);
        return;
      }
      if (typeof t.event == "string")
        for (const s of u.get(t.event) ?? [])
          try {
            s(t.data);
          } catch (o) {
            console.error("[filex-app-ui] listener failed", o);
          }
    }
  };
  async function g(e) {
    if (e.request === "save") {
      if (!a) {
        n.postMessage({ hid: e.hid, error: { code: "unavailable", message: "this app has no save handler" } });
        return;
      }
      try {
        const t = await y();
        n.postMessage({ hid: e.hid, result: t ?? { saved: !0 } });
      } catch (t) {
        const s = t instanceof p ? { code: t.code, message: t.message } : { code: "failed", message: String(t?.message ?? t) };
        n.postMessage({ hid: e.hid, error: s });
      }
      return;
    }
    n.postMessage({ hid: e.hid, error: { code: "unknown_method" } });
  }
  async function y() {
    if (!a) return null;
    const e = await a();
    return e == null ? null : l.save(e);
  }
  function r(e, t, s = []) {
    const o = ++d;
    return new Promise((w, h) => {
      i.set(o, { resolve: w, reject: h });
      try {
        n.postMessage({ id: o, method: e, params: t }, s);
      } catch (M) {
        i.delete(o), h(M);
      }
    });
  }
  function f(e, t) {
    r(e, t).catch((s) => console.warn("[filex-app-ui]", e, s));
  }
  async function m(e) {
    if (typeof e == "string") return { data: e, transfer: [] };
    if (e instanceof ArrayBuffer) return { data: e, transfer: [e] };
    if (ArrayBuffer.isView(e)) {
      const t = e.buffer.slice(e.byteOffset, e.byteOffset + e.byteLength);
      return { data: t, transfer: [t] };
    }
    if (typeof Blob < "u" && e instanceof Blob) {
      const t = await e.arrayBuffer();
      return { data: t, transfer: [t] };
    }
    if (typeof ReadableStream < "u" && e instanceof ReadableStream)
      return { data: e, transfer: [e] };
    throw new p({ code: "invalid", message: "save takes a string, a Blob, bytes or a ReadableStream" });
  }
  const l = {
    session: void 0,
    async open(e = 0) {
      const t = l.session?.files?.[e];
      if (!t) throw new p({ code: "not_found", message: `no file at ${e}` });
      const s = (o, w) => r("file.read", { index: e, as: o }).then((h) => h[w]);
      return {
        ...t,
        text: () => s("text", "text"),
        bytes: () => s("bytes", "bytes"),
        stream: () => s("stream", "stream"),
        save: (o, w) => l.save(o, { index: e, mime: w })
      };
    },
    async save(e, t = {}) {
      const s = await m(e);
      return r("file.save", { index: t.index ?? 0, data: s.data, mime: t.mime }, s.transfer);
    },
    async saveAs(e, t, s) {
      const o = await m(t);
      return r("file.saveAs", { name: e, data: o.data, mime: s }, o.transfer);
    },
    async download(e, t, s) {
      const o = await m(t);
      return r("ui.download", { name: e, data: o.data, mime: s }, o.transfer);
    },
    dirty(e) {
      f("ui.dirty", { dirty: !!e });
    },
    title(e) {
      f("ui.title", { text: String(e).slice(0, x.maxLineChars) });
    },
    toast(e, t) {
      f("ui.toast", { text: String(e).slice(0, x.maxLineChars), tone: t });
    },
    confirm(e) {
      return r("ui.confirm", typeof e == "string" ? { text: e } : e);
    },
    close() {
      f("ui.close");
    },
    copy(e) {
      return r("clipboard.write", { text: String(e) });
    },
    call(e, t) {
      return r("engine.call", { method: e, params: t });
    },
    submit(e, t) {
      return r("job.submit", { action: e, params: t });
    },
    state: {
      get(e) {
        return r("state.get", { key: e });
      },
      set(e, t) {
        return r("state.set", { key: e, value: t });
      }
    },
    license: {
      get() {
        return r("license.get");
      }
    },
    on(e, t) {
      let s = u.get(e);
      return s || u.set(e, s = /* @__PURE__ */ new Set()), s.add(t), () => s.delete(t);
    },
    onSave(e) {
      return a = e, () => {
        a === e && (a = null);
      };
    },
    request: r
  };
  return c.saveShortcut !== !1 && typeof document < "u" && document.addEventListener(
    "keydown",
    (e) => {
      (e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && (e.key === "s" || e.key === "S") && a && (e.preventDefault(), y().catch((t) => l.toast(String(t?.message ?? t), "error")));
    },
    !0
  ), l;
}
function v(n, c, d) {
  if (typeof document > "u") return;
  const i = document.documentElement;
  if (n) {
    i.dataset.theme = n.mode, i.style.colorScheme = n.mode;
    for (const [u, a] of Object.entries(n.tokens ?? {}))
      /^--fe-[a-z0-9-]+$/.test(u) && typeof a == "string" && i.style.setProperty(u, a);
  }
  c && (i.lang = c), d && (i.dir = d);
}
export {
  k as BRIDGE_VERSION,
  p as FilexError,
  S as HELLO,
  x as LIMITS,
  L as METHODS,
  E as PORT,
  A as connect,
  I as isHello,
  q as isKnownMethod,
  B as isPortMessage,
  j as isRequest
};
//# sourceMappingURL=filex-app-ui.js.map
