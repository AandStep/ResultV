const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("electronAPI", {
  getApiToken: () => ipcRenderer.sendSync("get-api-token"),
  isAdmin: () => ipcRenderer.sendSync("is-admin"),
  restartAsAdmin: () => ipcRenderer.send("restart-as-admin"),
});
