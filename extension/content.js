// content.js — recording buffer, UI overlay, and communication with service worker.

(function () {
  "use strict";

  const PREFIX = "__JF_WS__";

  // --- State ---
  let isRecording = false;
  let recordBuffer = [];
  let startedAt = null;

  // --- Inject page-context WebSocket interceptor ---
  function injectInterceptor() {
    const s = document.createElement("script");
    s.src = chrome.runtime.getURL("ws-interceptor.js");
    s.onload = function () {
      s.remove();
    };
    (document.head || document.documentElement).appendChild(s);
  }
  injectInterceptor();

  // --- Listen for intercepted messages from page context ---
  window.addEventListener("message", function (event) {
    if (!event.data || event.data.type !== PREFIX) return;
    if (!isRecording) return;
    recordBuffer.push({
      dir: event.data.dir,
      ts: event.data.ts,
      data: event.data.data,
    });
    updateMessageCount();
  });

  // --- UI ---
  function createOverlay() {
    const root = document.createElement("div");
    root.id = "jf-overlay";
    root.style.cssText =
      "position:fixed;top:12px;right:12px;z-index:2147483647;" +
      "font-family:system-ui,-apple-system,Segoe UI,Roboto,Arial,sans-serif;" +
      "font-size:12px;color:#fff;background:rgba(0,0,0,0.80);" +
      "border:1px solid rgba(255,255,255,0.18);border-radius:10px;" +
      "padding:10px;min-width:240px;user-select:none;";

    // Title
    const title = document.createElement("div");
    title.textContent = "JackFools";
    title.style.cssText = "font-weight:700;margin-bottom:8px;font-size:13px;";
    root.appendChild(title);

    // --- Token input ---
    const tokenRow = document.createElement("div");
    tokenRow.style.cssText = "display:flex;gap:4px;margin-bottom:6px;";

    const tokenInput = document.createElement("input");
    tokenInput.id = "jf-token";
    tokenInput.type = "text";
    tokenInput.placeholder = "X-JF-Token";
    tokenInput.style.cssText =
      "flex:1;padding:4px 6px;border-radius:5px;border:1px solid rgba(255,255,255,0.25);" +
      "background:rgba(255,255,255,0.1);color:#fff;font-size:10px;box-sizing:border-box;";
    tokenRow.appendChild(tokenInput);

    const tokenBtn = document.createElement("button");
    tokenBtn.type = "button";
    tokenBtn.textContent = "OK";
    tokenBtn.style.cssText =
      "padding:4px 8px;border-radius:5px;border:1px solid rgba(255,255,255,0.25);" +
      "background:rgba(80,180,80,0.4);color:#fff;cursor:pointer;font-size:10px;font-weight:600;";
    tokenBtn.addEventListener("click", function () {
      saveToken(tokenInput.value.trim());
    });
    tokenRow.appendChild(tokenBtn);
    root.appendChild(tokenRow);

    // Connection indicator
    const connStatus = document.createElement("div");
    connStatus.id = "jf-conn";
    connStatus.style.cssText = "margin-bottom:6px;font-size:10px;opacity:0.7;";
    root.appendChild(connStatus);

    // Load saved token
    chrome.storage.local.get("jf_settings", function (data) {
      const settings = data.jf_settings || {};
      if (settings.token) {
        tokenInput.value = settings.token;
        setConnStatus(true);
      } else {
        setConnStatus(false);
      }
      if (settings.port) {
        // port is stored but not shown in overlay — only token matters for user
      }
    });

    // --- Quick event button (original functionality) ---
    const quickBtn = document.createElement("button");
    quickBtn.type = "button";
    quickBtn.textContent = "Send event";
    quickBtn.style.cssText =
      "padding:4px 8px;border-radius:6px;border:1px solid rgba(255,255,255,0.25);" +
      "background:rgba(255,255,255,0.12);color:#fff;cursor:pointer;font-size:11px;margin-bottom:8px;";
    quickBtn.addEventListener("click", function () {
      sendQuickEvent().catch(function (e) {
        setStatus("error: " + String(e), true);
      });
    });
    root.appendChild(quickBtn);

    // Separator
    const sep = document.createElement("div");
    sep.style.cssText = "border-top:1px solid rgba(255,255,255,0.15);margin:6px 0;";
    root.appendChild(sep);

    // Recording label
    const recLabel = document.createElement("div");
    recLabel.textContent = "WebSocket Recording";
    recLabel.style.cssText = "font-weight:600;margin-bottom:6px;";
    root.appendChild(recLabel);

    // Status
    const status = document.createElement("div");
    status.id = "jf-status";
    status.textContent = "idle";
    status.style.cssText = "margin-bottom:6px;opacity:0.8;";
    root.appendChild(status);

    // Message count (hidden by default)
    const msgCount = document.createElement("div");
    msgCount.id = "jf-msg-count";
    msgCount.style.cssText = "margin-bottom:6px;display:none;font-size:11px;opacity:0.7;";
    root.appendChild(msgCount);

    // Record button
    const recBtn = document.createElement("button");
    recBtn.id = "jf-rec-btn";
    recBtn.type = "button";
    recBtn.textContent = "Start recording";
    recBtn.style.cssText =
      "width:100%;padding:6px 10px;border-radius:6px;border:1px solid rgba(255,255,255,0.25);" +
      "background:rgba(80,180,80,0.4);color:#fff;cursor:pointer;font-size:12px;font-weight:600;margin-bottom:6px;";
    recBtn.addEventListener("click", toggleRecording);
    root.appendChild(recBtn);

    // Save form (hidden by default)
    const form = document.createElement("div");
    form.id = "jf-save-form";
    form.style.cssText = "display:none;margin-top:6px;";

    const nameInput = document.createElement("input");
    nameInput.id = "jf-action-name";
    nameInput.type = "text";
    nameInput.placeholder = "Action name (e.g. vote_for_answer)";
    nameInput.style.cssText =
      "width:100%;padding:5px 8px;border-radius:5px;border:1px solid rgba(255,255,255,0.25);" +
      "background:rgba(255,255,255,0.1);color:#fff;font-size:11px;margin-bottom:5px;box-sizing:border-box;";
    form.appendChild(nameInput);

    const noteInput = document.createElement("textarea");
    noteInput.id = "jf-action-note";
    noteInput.placeholder = "Note (optional, e.g. available variants)";
    noteInput.rows = 3;
    noteInput.style.cssText =
      "width:100%;padding:5px 8px;border-radius:5px;border:1px solid rgba(255,255,255,0.25);" +
      "background:rgba(255,255,255,0.1);color:#fff;font-size:11px;margin-bottom:5px;resize:vertical;box-sizing:border-box;";
    form.appendChild(noteInput);

    const btnRow = document.createElement("div");
    btnRow.style.cssText = "display:flex;gap:6px;";

    const saveBtn = document.createElement("button");
    saveBtn.type = "button";
    saveBtn.textContent = "Save";
    saveBtn.style.cssText =
      "flex:1;padding:5px;border-radius:5px;border:1px solid rgba(255,255,255,0.25);" +
      "background:rgba(80,180,80,0.4);color:#fff;cursor:pointer;font-size:11px;font-weight:600;";
    saveBtn.addEventListener("click", saveRecording);
    btnRow.appendChild(saveBtn);

    const cancelBtn = document.createElement("button");
    cancelBtn.type = "button";
    cancelBtn.textContent = "Cancel";
    cancelBtn.style.cssText =
      "flex:1;padding:5px;border-radius:5px;border:1px solid rgba(255,255,255,0.25);" +
      "background:rgba(180,80,80,0.4);color:#fff;cursor:pointer;font-size:11px;";
    cancelBtn.addEventListener("click", cancelRecording);
    btnRow.appendChild(cancelBtn);

    form.appendChild(btnRow);
    root.appendChild(form);

    document.documentElement.appendChild(root);
  }

  function setStatus(text, isError) {
    const el = document.getElementById("jf-status");
    if (!el) return;
    el.textContent = text;
    el.style.color = isError ? "#ff6b6b" : "#fff";
  }

  function setConnStatus(hasToken) {
    const el = document.getElementById("jf-conn");
    if (!el) return;
    if (hasToken) {
      el.textContent = "● token set";
      el.style.color = "#6bffb3";
    } else {
      el.textContent = "○ no token";
      el.style.color = "#ff6b6b";
    }
  }

  function saveToken(token) {
    if (!token) {
      setConnStatus(false);
      return;
    }
    chrome.storage.local.get("jf_settings", function (data) {
      const settings = data.jf_settings || {};
      settings.token = token;
      chrome.storage.local.set({ jf_settings: settings }, function () {
        setConnStatus(true);
        setStatus("token saved");
      });
    });
  }

  function updateMessageCount() {
    const el = document.getElementById("jf-msg-count");
    if (!el) return;
    const sends = recordBuffer.filter(function (m) {
      return m.dir === "send";
    }).length;
    const recvs = recordBuffer.filter(function (m) {
      return m.dir === "recv";
    }).length;
    el.textContent = "Messages: " + recordBuffer.length + " (send: " + sends + ", recv: " + recvs + ")";
    el.style.display = "block";
  }

  function toggleRecording() {
    if (isRecording) {
      stopRecording();
    } else {
      startRecording();
    }
  }

  function startRecording() {
    isRecording = true;
    recordBuffer = [];
    startedAt = Date.now();

    const btn = document.getElementById("jf-rec-btn");
    if (btn) {
      btn.textContent = "Stop recording";
      btn.style.background = "rgba(220,60,60,0.6)";
    }
    setStatus("recording...");
    updateMessageCount();

    const form = document.getElementById("jf-save-form");
    if (form) form.style.display = "none";
  }

  function stopRecording() {
    isRecording = false;

    const btn = document.getElementById("jf-rec-btn");
    if (btn) {
      btn.textContent = "Start recording";
      btn.style.background = "rgba(80,180,80,0.4)";
    }
    setStatus("stopped — " + recordBuffer.length + " messages captured");

    const form = document.getElementById("jf-save-form");
    if (form) form.style.display = "block";

    const nameInput = document.getElementById("jf-action-name");
    if (nameInput) nameInput.focus();
  }

  function cancelRecording() {
    recordBuffer = [];
    startedAt = null;

    const form = document.getElementById("jf-save-form");
    if (form) form.style.display = "none";

    const msgCount = document.getElementById("jf-msg-count");
    if (msgCount) msgCount.style.display = "none";

    setStatus("idle");
  }

  async function saveRecording() {
    const nameInput = document.getElementById("jf-action-name");
    const noteInput = document.getElementById("jf-action-note");

    const actionName = (nameInput ? nameInput.value.trim() : "");
    if (!actionName) {
      setStatus("action name is required", true);
      return;
    }

    const payload = {
      action_name: actionName,
      note: noteInput ? noteInput.value.trim() : "",
      page_url: location.href,
      started_at: startedAt,
      stopped_at: Date.now(),
      messages: recordBuffer,
    };

    setStatus("saving...");

    try {
      const resp = await chrome.runtime.sendMessage({
        type: "JF_RECORDING",
        recording: payload,
      });
      if (resp && resp.ok) {
        setStatus("saved (" + recordBuffer.length + " msgs)");
        recordBuffer = [];
        startedAt = null;
        const form = document.getElementById("jf-save-form");
        if (form) form.style.display = "none";
        const msgCount = document.getElementById("jf-msg-count");
        if (msgCount) msgCount.style.display = "none";
        if (nameInput) nameInput.value = "";
        if (noteInput) noteInput.value = "";
      } else {
        setStatus("save failed: " + (resp && resp.error ? resp.error : "unknown"), true);
      }
    } catch (e) {
      setStatus("save error: " + String(e), true);
    }
  }

  // --- Original quick event functionality ---
  function buildEvent() {
    return {
      type: "page_state",
      url: location.href,
      ts: Date.now(),
      payload: { title: document.title },
    };
  }

  async function sendQuickEvent() {
    setStatus("sending event...");
    const event = buildEvent();
    const resp = await chrome.runtime.sendMessage({ type: "JF_EVENT", event: event });
    if (!resp || !resp.ok) {
      setStatus("event failed (" + (resp && resp.status ? resp.status : "no-status") + ")", true);
      return;
    }
    setStatus("event ok");
  }

  createOverlay();
})();
