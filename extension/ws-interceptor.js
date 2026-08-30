// ws-interceptor.js — injected into page context to monkey-patch WebSocket.
// Captures send/receive messages and forwards them to the content script via postMessage.
(function () {
  "use strict";

  const PREFIX = "__JF_WS__";

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
})();
