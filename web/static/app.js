function mountDrawioIframe(iframe, opts) {
  const drawio = opts.drawio || "";
  const xmlUrl = opts.xmlUrl || "";
  if (!iframe || !drawio || !xmlUrl) {
    return { isReady: () => false, load() {}, exportXml() {} };
  }
  let params = "embed=1&proto=json&spin=1&saveAndExit=0&noSaveBtn=1&noExitBtn=1";
  if (opts.lightbox) {
    params += "&lightbox=1&chrome=0&nav=1&layers=1";
  } else {
    params += "&ui=sketch&libraries=1";
  }
  const extra = drawio.startsWith("/") ? "&offline=1" : "";
  iframe.src = drawio + (drawio.includes("?") ? "&" : "?") + params + extra;

  let ready = false;
  window.addEventListener("message", (evt) => {
    if (evt.source !== iframe.contentWindow) return;
    let msg;
    try { msg = JSON.parse(evt.data); } catch { return; }
    if (msg.event === "init") {
      fetch(xmlUrl)
        .then((r) => r.text())
        .then((xml) => {
          iframe.contentWindow.postMessage(JSON.stringify({
            action: "load",
            xml,
            autosave: opts.autosave ? 1 : 0,
          }), "*");
          ready = true;
          if (opts.onReady) opts.onReady();
        });
    }
    if (opts.autosave && msg.event === "autosave" && msg.xml && opts.onAutosave) {
      opts.onAutosave(msg.xml);
    }
    if (opts.onEvent) opts.onEvent(msg);
  });

  return {
    isReady: () => ready,
    load(xml) {
      if (!iframe.contentWindow) return;
      iframe.contentWindow.postMessage(JSON.stringify({
        action: "load",
        xml,
        autosave: opts.autosave ? 1 : 0,
      }), "*");
    },
    exportXml() {
      if (!iframe.contentWindow) return;
      iframe.contentWindow.postMessage(JSON.stringify({ action: "export", format: "xml" }), "*");
    },
  };
}

(() => {
  const forms = document.querySelectorAll("form[data-wait-title]");
  if (!forms.length) return;

  const overlay = document.createElement("div");
  overlay.className = "wait-overlay";
  overlay.hidden = true;
  overlay.innerHTML =
    '<div class="wait-card">' +
      '<div class="wait-spinner" aria-hidden="true"></div>' +
      "<h2></h2>" +
      '<p class="wait-status" aria-live="polite"></p>' +
      '<p class="wait-hint"></p>' +
      '<p class="wait-elapsed"></p>' +
      '<p class="wait-error" hidden></p>' +
      '<div class="wait-actions" hidden>' +
        '<button type="button" class="wait-retry">Повторить</button>' +
        '<button type="button" class="wait-close">Закрыть</button>' +
      "</div>" +
    "</div>";
  document.body.appendChild(overlay);

  const titleEl = overlay.querySelector("h2");
  const statusEl = overlay.querySelector(".wait-status");
  const hintEl = overlay.querySelector(".wait-hint");
  const elapsedEl = overlay.querySelector(".wait-elapsed");
  const errorEl = overlay.querySelector(".wait-error");
  const actionsEl = overlay.querySelector(".wait-actions");

  let busy = false;
  let elapsedTimer = 0;
  let activeForm = null;

  function formatElapsed(ms) {
    const s = Math.max(0, Math.floor(ms / 1000));
    return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
  }

  function stopElapsed() {
    if (elapsedTimer) {
      clearInterval(elapsedTimer);
      elapsedTimer = 0;
    }
  }

  function setState(state, status, err) {
    overlay.classList.remove("wait-sending", "wait-waiting", "wait-done", "wait-failed");
    overlay.classList.add("wait-" + state);
    statusEl.textContent = status;
    if (err) {
      errorEl.hidden = false;
      errorEl.textContent = err;
      actionsEl.hidden = false;
    } else {
      errorEl.hidden = true;
      errorEl.textContent = "";
      actionsEl.hidden = true;
    }
  }

  function closeOverlay() {
    stopElapsed();
    busy = false;
    overlay.hidden = true;
    if (activeForm) {
      const btn = activeForm.querySelector('button[type="submit"]');
      if (btn) btn.disabled = false;
    }
  }

  async function errorFrom(res) {
    try {
      const html = await res.text();
      const doc = new DOMParser().parseFromString(html, "text/html");
      const flash = doc.querySelector(".flash.error");
      const msg = flash && flash.textContent.trim();
      if (msg) return msg;
    } catch (_) { /* ignore parse errors */ }
    return res.statusText || ("ошибка " + res.status);
  }

  async function run(form) {
    if (busy) return;
    busy = true;
    activeForm = form;
    const btn = form.querySelector('button[type="submit"]');
    if (btn) btn.disabled = true;

    titleEl.textContent = form.dataset.waitTitle || "Обработка";
    hintEl.textContent = form.dataset.waitHint || "";
    elapsedEl.textContent = "0:00";
    overlay.hidden = false;
    setState("sending", "Отправляем…");

    const started = Date.now();
    const pending = fetch(form.action, { method: "POST", body: new FormData(form), redirect: "follow" });
    setState("waiting", "Ждём ответ модели");
    stopElapsed();
    elapsedTimer = setInterval(() => {
      elapsedEl.textContent = formatElapsed(Date.now() - started);
    }, 250);

    try {
      const res = await pending;
      stopElapsed();
      elapsedEl.textContent = formatElapsed(Date.now() - started);
      if (!res.ok) {
        busy = false;
        if (btn) btn.disabled = false;
        setState("failed", "Не удалось", await errorFrom(res));
        return;
      }
      setState("done", "Готово");
      const next = res.url || form.action;
      setTimeout(() => { window.location.assign(next); }, 400);
    } catch (err) {
      stopElapsed();
      elapsedEl.textContent = formatElapsed(Date.now() - started);
      busy = false;
      if (btn) btn.disabled = false;
      setState("failed", "Не удалось", (err && err.message) || "нет сети");
    }
  }

  overlay.querySelector(".wait-retry").addEventListener("click", () => {
    if (activeForm) run(activeForm);
  });
  overlay.querySelector(".wait-close").addEventListener("click", closeOverlay);

  forms.forEach((form) => {
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      run(form);
    });
  });
})();

(() => {
  const root = document.querySelector(".session");
  if (!root) return;

  const sessionId = root.dataset.session;
  const chat = document.getElementById("chat");
  const form = document.getElementById("chat-form");
  const timerEl = document.getElementById("timer");
  const usageEl = document.getElementById("usage");
  const iframe = document.getElementById("board");
  const showBtn = document.getElementById("show-board");

  function appendMsg(role, text) {
    const art = document.createElement("article");
    art.className = "msg " + role;
    const title = role === "user" ? "Вы" : role === "assistant" ? "Интервьюер" : "Система";
    art.innerHTML = "<h3></h3><div class=\"body\"></div>";
    art.querySelector("h3").textContent = title;
    art.querySelector(".body").textContent = text;
    chat.appendChild(art);
    chat.scrollTop = chat.scrollHeight;
    return art.querySelector(".body");
  }

  function ruCount(n, one, few, many) {
    const n100 = n % 100;
    let word = many;
    if (n100 < 11 || n100 > 14) {
      const n10 = n % 10;
      if (n10 === 1) word = one;
      else if (n10 >= 2 && n10 <= 4) word = few;
    }
    return n + " " + word;
  }

  function appendBoardEvent(dump, nodes, edges) {
    const art = document.createElement("article");
    art.className = "msg event";
    const h3 = document.createElement("h3");
    h3.textContent = "Доска показана интервьюеру";
    const lead = document.createElement("p");
    lead.className = "event-lead";
    lead.textContent = "Интервьюер не видит картинку — только текстовую схему из узлов и стрелок.";
    const meta = document.createElement("p");
    meta.className = "meta";
    if (typeof nodes === "number" && typeof edges === "number") {
      meta.textContent = ruCount(nodes, "узел", "узла", "узлов") + ", " + ruCount(edges, "связь", "связи", "связей");
    } else {
      meta.textContent = String(dump || "").split("\n")[0] || "текстовая схема";
    }
    art.appendChild(h3);
    art.appendChild(lead);
    art.appendChild(meta);
    if (dump) {
      const details = document.createElement("details");
      const summary = document.createElement("summary");
      summary.textContent = "Что увидел интервьюер";
      const pre = document.createElement("pre");
      pre.className = "board-dump";
      pre.textContent = dump;
      details.appendChild(summary);
      details.appendChild(pre);
      art.appendChild(details);
    }
    chat.appendChild(art);
    chat.scrollTop = chat.scrollHeight;
  }

  async function readSSE(res, onEvent) {
    const reader = res.body.getReader();
    const dec = new TextDecoder();
    let buf = "";
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += dec.decode(value, { stream: true });
      const parts = buf.split("\n\n");
      buf = parts.pop();
      for (const block of parts) {
        const line = block.split("\n").find((l) => l.startsWith("data: "));
        if (!line) continue;
        let ev;
        try { ev = JSON.parse(line.slice(6)); } catch { continue; }
        onEvent(ev);
      }
    }
  }

  if (form) {
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      const ta = form.querySelector("textarea");
      const content = ta.value.trim();
      if (!content) return;
      ta.value = "";
      appendMsg("user", content);
      const body = appendMsg("assistant", "");
      const res = await fetch("/sessions/" + sessionId + "/messages", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
        body: JSON.stringify({ content }),
      });
      if (!res.ok || !res.body) {
        body.textContent = "Ошибка отправки.";
        return;
      }
      await readSSE(res, (ev) => {
        if (ev.type === "token") body.textContent += ev.text;
        if (ev.type === "usage" && usageEl) {
          usageEl.textContent = ev.label || "";
        }
        if (ev.type === "error") body.textContent += "\n" + ev.message;
        chat.scrollTop = chat.scrollHeight;
      });
    });
  }

  const minutes = Number(root.dataset.timer || 0);
  const started = Number(root.dataset.started || 0);
  if (timerEl && minutes > 0 && started > 0) {
    const tick = () => {
      const left = minutes * 60 - (Math.floor(Date.now() / 1000) - started);
      if (left <= 0) {
        timerEl.textContent = "время вышло";
        return;
      }
      const m = Math.floor(left / 60);
      const s = String(left % 60).padStart(2, "0");
      timerEl.textContent = m + ":" + s;
      requestAnimationFrame(() => setTimeout(tick, 500));
    };
    tick();
  }

  if (!iframe) return;

  let pendingShow = false;
  let showing = false;
  const board = mountDrawioIframe(iframe, {
    drawio: root.dataset.drawio,
    xmlUrl: root.dataset.xmlUrl,
    autosave: true,
    onAutosave(xml) {
      fetch("/sessions/" + sessionId + "/board", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ xml, show: false }),
      });
    },
    onEvent(msg) {
      if (msg.event !== "export" || !pendingShow) return;
      const xml = msg.xml || msg.data;
      if (!xml) return;
      pendingShow = false;
      showBoard(xml);
    },
  });

  async function showBoard(xml) {
    if (showing) return;
    showing = true;
    const label = showBtn ? showBtn.textContent : "";
    if (showBtn) {
      showBtn.disabled = true;
      showBtn.textContent = "Показываем…";
    }
    try {
      const res = await fetch("/sessions/" + sessionId + "/board", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
        body: JSON.stringify({ xml, show: true }),
      });
      if (!res.ok || !res.body) {
        appendMsg("system", "Ошибка отправки доски.");
        return;
      }
      let assistantBody = null;
      await readSSE(res, (ev) => {
        if (ev.type === "shown") {
          appendBoardEvent(ev.dump, ev.nodes, ev.edges);
        }
        if (ev.type === "token") {
          if (!assistantBody) assistantBody = appendMsg("assistant", "");
          assistantBody.textContent += ev.text;
        }
        if (ev.type === "usage" && usageEl) {
          usageEl.textContent = ev.label || "";
        }
        if (ev.type === "error") {
          if (!assistantBody) assistantBody = appendMsg("assistant", "");
          assistantBody.textContent += (assistantBody.textContent ? "\n" : "") + ev.message;
        }
        chat.scrollTop = chat.scrollHeight;
      });
    } catch (err) {
      appendMsg("system", (err && err.message) || "нет сети");
    } finally {
      showing = false;
      if (showBtn) {
        showBtn.disabled = false;
        showBtn.textContent = label;
      }
    }
  }

  if (showBtn) {
    showBtn.addEventListener("click", () => {
      if (!board.isReady() || showing || pendingShow) return;
      pendingShow = true;
      board.exportXml();
    });
  }

  const uploadForm = root.querySelector("form.upload");
  if (uploadForm) {
    uploadForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const input = uploadForm.querySelector('input[type="file"]');
      const file = input && input.files && input.files[0];
      if (!file) return;
      const xml = await file.text();
      if (!xml.trim()) return;
      await fetch("/sessions/" + sessionId + "/board", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ xml, show: false }),
      });
      if (board.isReady()) board.load(xml);
    });
  }
})();

(() => {
  document.querySelectorAll("iframe[data-drawio][data-xml-url]").forEach((el) => {
    if (el.id === "board") return;
    mountDrawioIframe(el, {
      drawio: el.dataset.drawio,
      xmlUrl: el.dataset.xmlUrl,
      lightbox: true,
    });
  });
})();
