const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("electronAPI", {
  getApiToken: () => ipcRenderer.sendSync("get-api-token"),
});
