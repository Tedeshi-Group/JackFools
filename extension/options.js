const STORAGE_KEY = "jf_settings";

function $(id) {
  return document.getElementById(id);
}

async function load() {
  const data = await chrome.storage.local.get(STORAGE_KEY);
  const settings = data[STORAGE_KEY] || {};
  $("port").value = settings.port ?? "27124";
  $("token").value = settings.token ?? "";
}

async function save() {
  const portRaw = $("port").value.trim();
  const token = $("token").value.trim();

  const port = Number(portRaw || "27124");
  if (!Number.isInteger(port) || port <= 0 || port > 65535) {
    setStatus("Invalid port", true);
    return;
  }
  if (!token) {
    setStatus("Token is required", true);
    return;
  }

  await chrome.storage.local.set({
    [STORAGE_KEY]: {
      port: String(port),
      token
    }
  });

  setStatus("Saved", false);
}

function setStatus(text, isError) {
  const el = $("status");
  el.textContent = text;
  el.style.color = isError ? "#b00020" : "#0a7a0a";
}

document.addEventListener("DOMContentLoaded", () => {
  load().catch((e) => setStatus(String(e), true));
  $("save").addEventListener("click", () => save().catch((e) => setStatus(String(e), true)));
});



