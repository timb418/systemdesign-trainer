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
          if (ev.type === "token") body.textContent += ev.text;
          if (ev.type === "usage" && usageEl) {
            usageEl.textContent = ev.label || "";
          }
          if (ev.type === "error") body.textContent += "\n" + ev.message;
          chat.scrollTop = chat.scrollHeight;
        }
      }
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

  const drawio = root.dataset.drawio;
  const params = "embed=1&proto=json&spin=1&ui=sketch&libraries=1&saveAndExit=0&noSaveBtn=1&noExitBtn=1";
  const extra = drawio.startsWith("/") ? "&offline=1" : "";
  iframe.src = drawio + (drawio.includes("?") ? "&" : "?") + params + extra;

  let ready = false;
  window.addEventListener("message", (evt) => {
    if (evt.source !== iframe.contentWindow) return;
    let msg;
    try { msg = JSON.parse(evt.data); } catch { return; }
    if (msg.event === "init") {
      fetch(root.dataset.xmlUrl)
        .then((r) => r.text())
        .then((xml) => {
          iframe.contentWindow.postMessage(JSON.stringify({ action: "load", xml, autosave: 1 }), "*");
          ready = true;
        });
    }
    if (msg.event === "autosave" && msg.xml) {
      fetch("/sessions/" + sessionId + "/board", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ xml: msg.xml, show: false }),
      });
    }
    if (msg.event === "export" && msg.xml && pendingShow) {
      pendingShow = false;
      fetch("/sessions/" + sessionId + "/board", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ xml: msg.xml, show: true }),
      }).then(async (r) => {
        const data = await r.json().catch(() => ({}));
        if (data.dump) appendMsg("system", "Показана доска:\n" + data.dump);
        if (data.reply) appendMsg("assistant", data.reply);
      });
    }
  });

  let pendingShow = false;
  if (showBtn) {
    showBtn.addEventListener("click", () => {
      if (!ready) return;
      pendingShow = true;
      iframe.contentWindow.postMessage(JSON.stringify({ action: "export", format: "xml" }), "*");
    });
  }
})();
