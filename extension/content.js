// content.js — recording buffer, UI overlay, and communication with service worker.

(function () {
  "use strict";

  const PREFIX = "__JF_WS__";

  // --- State ---
  let isRecording = false;
  let recordBuffer = [];
  let startedAt = null;

  // --- TD2 Bot State ---
  let currentQuestion = null; // { prompt, choices, countGroupKey, type }
  let questionBankCount = 0;

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

    // Recording buffer (always active)
    if (isRecording) {
      recordBuffer.push({
        dir: event.data.dir,
        ts: event.data.ts,
        data: event.data.data,
      });
      updateMessageCount();
    }

    // TD2 bot processing (only incoming messages)
    if (event.data.dir === "recv" && window.__JF_TD2_PARSER__) {
      handleTD2Message(event.data.data);
    }
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

    // --- TD2 Bot section ---
    const botLabel = document.createElement("div");
    botLabel.textContent = "TD2 Audience Bot";
    botLabel.style.cssText = "font-weight:600;margin-bottom:6px;";
    root.appendChild(botLabel);

    // Last question display
    const lastQ = document.createElement("div");
    lastQ.id = "jf-last-question";
    lastQ.textContent = "No active question";
    lastQ.style.cssText = "font-size:10px;opacity:0.5;margin-bottom:4px;word-break:break-word;";
    root.appendChild(lastQ);

    // Bank count
    const bankCount = document.createElement("div");
    bankCount.id = "jf-bank-count";
    bankCount.textContent = "Bank: 0 questions";
    bankCount.style.cssText = "font-size:10px;opacity:0.7;margin-bottom:4px;";
    root.appendChild(bankCount);

    // Bot status
    const botStatus = document.createElement("div");
    botStatus.id = "jf-bot-status";
    botStatus.style.cssText = "font-size:10px;opacity:0.8;color:#6bffb3;min-height:14px;";
    root.appendChild(botStatus);

    // Separator before recording
    const sep2 = document.createElement("div");
    sep2.style.cssText = "border-top:1px solid rgba(255,255,255,0.15);margin:6px 0;";
    root.appendChild(sep2);

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

  // ===== TD2 Bot Logic =====

  function handleTD2Message(raw) {
    const parser = window.__JF_TD2_PARSER__;
    const ev = parser.classify(raw);
    if (!ev) return;

    // Handle arrays (multiple textDescription events in one message)
    if (Array.isArray(ev)) {
      for (const e of ev) handleSingleTD2Event(e);
      return;
    }
    handleSingleTD2Event(ev);
  }

  function handleSingleTD2Event(ev) {
    switch (ev.type) {
      case "regular_question":
      case "final_round_question":
      case "death_room_vote":
        currentQuestion = {
          prompt: ev.prompt,
          choices: ev.choices,
          countGroupKey: ev.countGroupKey,
          type: ev.type,
        };
        updateQuestionDisplay();
        autoVote(ev);
        break;

      case "voting_closed":
        currentQuestion = null;
        updateQuestionDisplay();
        break;

      case "correct_answer":
      case "correct_answers":
        if (currentQuestion) {
          learnAnswer(currentQuestion, ev.answerTexts);
        }
        break;
    }
  }

  // --- Auto-vote (T4) ---
  function autoVote(ev) {
    const vote = { name: ev.countGroupKey || "TriviaDeath2AudienceChoice", times: 1 };

    if (ev.type === "death_room_vote") {
      // Death room: always random
      vote.vote = String(Math.floor(Math.random() * ev.choices.length));
    } else if (ev.type === "final_round_question") {
      // Final round: only vote if we know the answer
      const known = lookupAnswerSync(ev.prompt);
      if (known && known.length > 0) {
        vote.vote = known.join(",");
      } else {
        return; // Skip unknown final round questions
      }
    } else {
      // Regular question: vote known or random
      const known = lookupAnswerSync(ev.prompt);
      if (known && known.length > 0) {
        vote.vote = String(known[0]);
      } else {
        vote.vote = String(Math.floor(Math.random() * ev.choices.length));
      }
    }

    sendVote(vote);
    setBotStatus("voted: " + vote.vote);
  }

  function lookupAnswerSync(prompt) {
    // Synchronous lookup requires the bank to be cached
    if (!window.__JF_TD2_BANK__) return null;
    const bank = window.__JF_TD2_BANK__;
    for (const q of bank) {
      if (q.prompt === prompt) {
        return q.answers.map(function (a) { return a.index; });
      }
    }
    return null;
  }

  function sendVote(vote) {
    window.dispatchEvent(new CustomEvent("__JF_VOTE__", { detail: vote }));
  }

  // --- Teaching loop (T5) ---
  function learnAnswer(question, answerTexts) {
    if (!answerTexts || answerTexts.length === 0) return;

    const answers = [];
    for (const text of answerTexts) {
      const idx = question.choices.indexOf(text);
      if (idx !== -1) {
        answers.push({ text: text, index: idx });
      }
    }
    if (answers.length === 0) return;

    // Send to server
    chrome.runtime.sendMessage({
      type: "JF_QUESTION_STORE",
      prompt: question.prompt,
      answers: answers,
    }, function (resp) {
      if (resp && resp.ok) {
        // Update local cache
        upsertLocalBank(question.prompt, answers);
        setBotStatus("learned: " + question.prompt.substring(0, 40));
        updateBankCount();
      }
    });
  }

  function upsertLocalBank(prompt, answers) {
    if (!window.__JF_TD2_BANK__) window.__JF_TD2_BANK__ = [];
    const bank = window.__JF_TD2_BANK__;
    for (let i = 0; i < bank.length; i++) {
      if (bank[i].prompt === prompt) {
        // Merge answers
        for (const a of answers) {
          const exists = bank[i].answers.some(function (e) { return e.text === a.text; });
          if (!exists) bank[i].answers.push(a);
        }
        bank[i].seen_count = (bank[i].seen_count || 0) + 1;
        bank[i].last_seen = Date.now();
        return;
      }
    }
    bank.push({
      prompt: prompt,
      answers: answers,
      seen_count: 1,
      last_seen: Date.now(),
    });
  }

  // --- Question bank loading ---
  function loadQuestionBank() {
    chrome.runtime.sendMessage({ type: "JF_QUESTIONS_LIST" }, function (resp) {
      if (resp && resp.ok && resp.questions) {
        window.__JF_TD2_BANK__ = resp.questions;
        questionBankCount = resp.questions.length;
        updateBankCount();
      }
    });
  }

  // --- UI additions ---
  function updateQuestionDisplay() {
    const el = document.getElementById("jf-last-question");
    if (!el) return;
    if (!currentQuestion) {
      el.textContent = "No active question";
      el.style.opacity = "0.5";
      return;
    }
    const known = lookupAnswerSync(currentQuestion.prompt);
    const icon = known ? "\u2705" : "\u2753";
    const label = known ? "known" : "unknown";
    el.textContent = icon + " " + currentQuestion.prompt.substring(0, 50) + " [" + label + "]";
    el.style.opacity = "1";
  }

  function updateBankCount() {
    const el = document.getElementById("jf-bank-count");
    if (!el) return;
    el.textContent = "Bank: " + questionBankCount + " questions";
  }

  function setBotStatus(text) {
    const el = document.getElementById("jf-bot-status");
    if (!el) return;
    el.textContent = text;
    el.style.opacity = "0.8";
    clearTimeout(el._timer);
    el._timer = setTimeout(function () {
      el.textContent = "";
    }, 3000);
  }

  createOverlay();
  loadQuestionBank();
})();
