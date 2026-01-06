const STORAGE_KEY = "jf_settings";

async function getSettings() {
  const data = await chrome.storage.local.get(STORAGE_KEY);
  const settings = data[STORAGE_KEY] || {};
  return {
    port: settings.port || "27124",
    token: settings.token || ""
  };
}

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (!msg || msg.type !== "JF_EVENT") {
    return;
  }

  (async () => {
    const { port, token } = await getSettings();
    if (!token) {
      sendResponse({ ok: false, error: "token_not_set" });
      return;
    }

    const url = `http://127.0.0.1:${port}/v1/event`;
    const res = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-JF-Token": token
      },
      body: JSON.stringify(msg.event)
    });

    const data = await res.json().catch(() => null);
    sendResponse({ ok: res.ok, status: res.status, data });
  })().catch((e) => {
    sendResponse({ ok: false, error: String(e) });
  });

  return true;
});



