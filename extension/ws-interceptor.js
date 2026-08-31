// ws-interceptor.js — injected into page context to monkey-patch WebSocket.
// Captures send/receive messages and forwards them to the content script via postMessage.
(function () {
  "use strict";

  const PREFIX = "__JF_WS__";
  let seqCounter = 1;

  // Track all open WebSocket instances. Cleaned up on close/error.
  const openSockets = new Set();

  function registerSocket(ws) {
    openSockets.add(ws);
    // Remove from tracking on close or error so stale refs don't accumulate.
    const cleanup = function () {
      openSockets.delete(ws);
    };
    ws.addEventListener("close", cleanup, { once: true });
    ws.addEventListener("error", cleanup, { once: true });
  }

  // Return the best WebSocket for sending: prefer the most recently opened OPEN
  // socket, fall back to any OPEN socket, return null if none available.
  function getOpenSocket() {
    // openSockets iterates in insertion order (most recent last). Walk backwards
    // to prefer the latest OPEN socket.
    let best = null;
    for (const ws of openSockets) {
      if (ws.readyState === WebSocket.OPEN) {
        best = ws; // Keep overwriting — last OPEN wins.
      }
    }
    return best;
  }

  function postCapture(dir, data) {
    let str;
    if (typeof data === "string") {
      str = data;
    } else if (data instanceof Blob) {
      data.text().then(function (text) {
        window.postMessage({ type: PREFIX, dir: dir, ts: Date.now(), data: text }, "*");
      });
      return;
    } else if (data instanceof ArrayBuffer) {
      try {
        str = new TextDecoder().decode(data);
      } catch (e) {
        str = "[binary " + data.byteLength + " bytes]";
      }
    } else if (data && data.buffer instanceof ArrayBuffer) {
      try {
        str = new TextDecoder().decode(data);
      } catch (e) {
        str = "[binary " + data.byteLength + " bytes]";
      }
    } else {
      str = String(data);
    }
    window.postMessage({ type: PREFIX, dir: dir, ts: Date.now(), data: str }, "*");
  }

  // Wrap WebSocket.prototype.send
  const origSend = WebSocket.prototype.send;
  WebSocket.prototype.send = function (data) {
    registerSocket(this);
    postCapture("send", data);
    return origSend.call(this, data);
  };

  // Wrap onmessage setter to intercept received messages
  const origDescriptor = Object.getOwnPropertyDescriptor(WebSocket.prototype, "onmessage");
  if (origDescriptor && origDescriptor.set) {
    Object.defineProperty(WebSocket.prototype, "onmessage", {
      configurable: true,
      enumerable: true,
      get: origDescriptor.get
        ? function () {
            return origDescriptor.get.call(this);
          }
        : undefined,
      set: function (handler) {
        const wrappedHandler = function (event) {
          postCapture("recv", event.data);
          if (handler) handler.call(this, event);
        };
        return origDescriptor.set.call(this, wrappedHandler);
      },
    });
  }

  // Wrap addEventListener to intercept "message" listeners
  const origAddEventListener = WebSocket.prototype.addEventListener;
  WebSocket.prototype.addEventListener = function (type, listener, options) {
    if (type === "message") {
      const wrappedListener = function (event) {
        postCapture("recv", event.data);
        if (listener) {
          if (typeof listener === "function") {
            listener.call(this, event);
          } else if (listener.handleEvent) {
            listener.handleEvent(event);
          }
        }
      };
      return origAddEventListener.call(this, type, wrappedListener, options);
    }
    return origAddEventListener.call(this, type, listener, options);
  };

  // Track every new WebSocket instance on construction and on first send.
  const OrigWebSocket = WebSocket;
  window.WebSocket = function (url, protocols) {
    const ws = protocols !== undefined
      ? new OrigWebSocket(url, protocols)
      : new OrigWebSocket(url);
    registerSocket(ws);
    return ws;
  };
  window.WebSocket.prototype = OrigWebSocket.prototype;
  window.WebSocket.CONNECTING = OrigWebSocket.CONNECTING;
  window.WebSocket.OPEN = OrigWebSocket.OPEN;
  window.WebSocket.CLOSING = OrigWebSocket.CLOSING;
  window.WebSocket.CLOSED = OrigWebSocket.CLOSED;

  // Listen for vote requests from content.js
  window.addEventListener("__JF_VOTE__", function (event) {
    var ws = getOpenSocket();
    if (!ws) {
      console.log("[JF-WS] vote dropped: no open WebSocket (tracked:", openSockets.size, ")");
      return;
    }
    var detail = event.detail;
    if (!detail || !detail.name) return;
    var payload = JSON.stringify({
      seq: seqCounter++,
      opcode: "audience/count-group/increment",
      params: {
        name: detail.name,
        vote: String(detail.vote),
        times: detail.times || 1,
      },
    });
    console.log("[JF-WS] sending vote:", payload);
    ws.send(payload);
  });
})();
