const EventEmitter = require("events");

class StateStore extends EventEmitter {
  constructor() {
    super();
    this.state = {
      isConnected: false,
      activeProxy: null,
      bytesSent: 0,
      bytesReceived: 0,
      speedReceived: 0,
      speedSent: 0,
      isProxyDead: false,
      killSwitch: false,
      adblock: false,
      uiProxies: [], // proxies cached from UI
      lastTickStats: { received: 0, sent: 0, time: Date.now() },
      sessionStartStats: { received: 0, sent: 0 },
    };
  }

  getState() {
    return this.state;
  }

  update(partialState) {
    let changed = false;
    for (const key in partialState) {
      if (this.state[key] !== partialState[key]) {
        this.state[key] = partialState[key];
        changed = true;
      }
    }
    if (changed) {
      this.emit("change", this.state);
    }
  }
}

module.exports = new StateStore();
