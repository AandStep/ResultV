const { BrowserWindow, nativeImage, app, session } = require("electron");
const path = require("path");

class WindowManager {
  constructor() {
    this.mainWindow = null;
  }

  create() {
    const isDev = process.env.NODE_ENV === "development";
    const iconPath = isDev
      ? path.join(__dirname, "../../public", "logo.png")
      : path.join(__dirname, "../../dist", "logo.png");

    this.mainWindow = new BrowserWindow({
      width: 1050,
      height: 780,
      icon: nativeImage.createFromPath(iconPath),
      autoHideMenuBar: true,
      webPreferences: {
        nodeIntegration: false,
        contextIsolation: true,
        sandbox: true,
        preload: path.join(__dirname, "preload.cjs"),
      },
    });

    // CSP-заголовки (только в production — в dev Vite требует inline-скрипты и WebSocket для HMR)
    if (!isDev) {
      session.defaultSession.webRequest.onHeadersReceived(
        (details, callback) => {
          callback({
            responseHeaders: {
              ...details.responseHeaders,
              "Content-Security-Policy": [
                [
                  "default-src 'self'",
                  "script-src 'self'",
                  "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
                  "font-src 'self' https://fonts.gstatic.com",
                  "img-src 'self' data: https://flagcdn.com https://cdnjs.cloudflare.com",
                  "connect-src 'self' http://127.0.0.1:14080",
                ].join("; "),
              ],
            },
          });
        },
      );
    }

    this.mainWindow.loadURL(
      isDev
        ? "http://localhost:5173"
        : `file://${path.join(__dirname, "../../dist/index.html")}`,
    );

    this.mainWindow.on("close", (event) => {
      if (!app.isQuitting) {
        event.preventDefault();
        this.mainWindow.hide();
      }
      return false;
    });
  }

  show() {
    if (this.mainWindow) {
      if (!this.mainWindow.isVisible()) this.mainWindow.show();
      if (this.mainWindow.isMinimized()) this.mainWindow.restore();
      this.mainWindow.focus();
    }
  }

  toggle() {
    if (this.mainWindow) {
      this.mainWindow.isVisible()
        ? this.mainWindow.hide()
        : this.mainWindow.show();
    }
  }
}

module.exports = WindowManager;
