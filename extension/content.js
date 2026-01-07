function createOverlay() {
  const root = document.createElement("div");
  root.id = "jf-overlay";
  root.style.position = "fixed";
  root.style.top = "12px";
  root.style.right = "12px";
  root.style.zIndex = "2147483647";
  root.style.fontFamily = "system-ui, -apple-system, Segoe UI, Roboto, Arial, sans-serif";
  root.style.fontSize = "12px";
  root.style.color = "#fff";
  root.style.background = "rgba(0,0,0,0.75)";
  root.style.border = "1px solid rgba(255,255,255,0.18)";
  root.style.borderRadius = "10px";
  root.style.padding = "10px";
  root.style.minWidth = "220px";
  root.style.userSelect = "none";

  const title = document.createElement("div");
  title.textContent = "JackFools (local)";
  title.style.fontWeight = "700";
  title.style.marginBottom = "6px";

  const status = document.createElement("div");
  status.id = "jf-status";
  status.textContent = "idle";

  const btn = document.createElement("button");
  btn.type = "button";
  btn.textContent = "Send event";
  btn.style.marginTop = "8px";
  btn.style.padding = "6px 10px";
  btn.style.borderRadius = "8px";
  btn.style.border = "1px solid rgba(255,255,255,0.25)";
  btn.style.background = "rgba(255,255,255,0.12)";
  btn.style.color = "#fff";
  btn.style.cursor = "pointer";

  btn.addEventListener("click", () => {
    sendEvent().catch((e) => setStatus(`error: ${String(e)}`));
  });

  root.appendChild(title);
  root.appendChild(status);
  root.appendChild(btn);

  document.documentElement.appendChild(root);
}

function setStatus(text) {
  const el = document.getElementById("jf-status");
  if (el) el.textContent = text;
}

function buildEvent() {
  return {
    type: "page_state",
    url: location.href,
    ts: Date.now(),
    payload: {
      title: document.title
    }
  };
}

async function sendEvent() {
  setStatus("sending...");
  const event = buildEvent();

  const resp = await chrome.runtime.sendMessage({ type: "JF_EVENT", event });
  if (!resp || !resp.ok) {
    setStatus(`failed (${resp && resp.status ? resp.status : "no-status"})`);
    return;
  }
  setStatus("ok");
}

createOverlay();




