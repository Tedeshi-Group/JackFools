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
  if (!msg) return;

  if (msg.type === "JF_EVENT") {
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
  }

  if (msg.type === "JF_RECORDING") {
    (async () => {
      const { port, token } = await getSettings();
      if (!token) {
        sendResponse({ ok: false, error: "token_not_set" });
        return;
      }

      const url = `http://127.0.0.1:${port}/v1/recording`;
      const res = await fetch(url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-JF-Token": token
        },
        body: JSON.stringify(msg.recording)
      });

      const data = await res.json().catch(() => null);
      sendResponse({ ok: res.ok, status: res.status, data });
    })().catch((e) => {
      sendResponse({ ok: false, error: String(e) });
    });

    return true;
  }

  if (msg.type === "JF_QUESTION_STORE") {
    console.log("[JF-SW] JF_QUESTION_STORE:", msg.prompt.substring(0, 60), "answers:", msg.answers.length);
    (async () => {
      const { port, token } = await getSettings();
      if (!token) {
        console.log("[JF-SW] JF_QUESTION_STORE: no token");
        sendResponse({ ok: false, error: "token_not_set" });
        return;
      }

      const url = `http://127.0.0.1:${port}/v1/questions`;
      console.log("[JF-SW] JF_QUESTION_STORE: POST", url);
      const res = await fetch(url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-JF-Token": token
        },
        body: JSON.stringify({ prompt: msg.prompt, answers: msg.answers })
      });

      const data = await res.json().catch(() => null);
      console.log("[JF-SW] JF_QUESTION_STORE: response", res.status, JSON.stringify(data));
      sendResponse({ ok: res.ok, status: res.status, data });
    })().catch((e) => {
      console.log("[JF-SW] JF_QUESTION_STORE error:", String(e));
      sendResponse({ ok: false, error: String(e) });
    });

    return true;
  }

  if (msg.type === "JF_QUESTIONS_LIST") {
    (async () => {
      const { port, token } = await getSettings();
      if (!token) {
        sendResponse({ ok: false, error: "token_not_set" });
        return;
      }

      const url = `http://127.0.0.1:${port}/v1/questions`;
      const res = await fetch(url, {
        method: "GET",
        headers: {
          "X-JF-Token": token
        }
      });

      const data = await res.json().catch(() => null);
      if (data && data.ok) {
        sendResponse({ ok: true, questions: data.questions || [] });
      } else {
        sendResponse({ ok: false, error: data ? data.error : "request failed" });
      }
    })().catch((e) => {
      sendResponse({ ok: false, error: String(e) });
    });

    return true;
  }

  if (msg.type === "JF_QUESTION_LOOKUP") {
    (async () => {
      const { port, token } = await getSettings();
      if (!token) {
        sendResponse({ ok: false, error: "token_not_set" });
        return;
      }

      const url = `http://127.0.0.1:${port}/v1/questions/lookup?prompt=${encodeURIComponent(msg.prompt)}`;
      const res = await fetch(url, {
        method: "GET",
        headers: {
          "X-JF-Token": token
        }
      });

      const data = await res.json().catch(() => null);
      sendResponse({ ok: res.ok, data });
    })().catch((e) => {
      sendResponse({ ok: false, error: String(e) });
    });

    return true;
  }
});
