const { BrowserWindow, nativeImage, app } = require("electron");
const path = require("path");

class WindowManager {
  constructor() {
    this.mainWindow = null;
  }

  create() {
    const iconPath =
      process.env.NODE_ENV === "development"
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
        preload: path.join(__dirname, "preload.cjs"),
      },
    });

    this.mainWindow.loadURL(
      process.env.NODE_ENV === "development"
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
