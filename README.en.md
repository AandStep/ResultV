<p align="center">
  <img src="./public/logo.png" width="128">
</p>

<h1 align="center">ResultProxy</h1>

<p align="center">
  <b>Cross-platform proxy application with built-in ad blocker.</b><br>
  More than just a proxy — your reliable tool to bypass restrictions.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Version-2.1.0-blue.svg" alt="Version">
  <img src="https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-orange.svg" alt="Platform">
  <img src="https://img.shields.io/badge/Frontend-React-61dafb.svg" alt="Frontend">
  <img src="https://img.shields.io/badge/Backend-Electron-47848f.svg" alt="Backend">
</p>

<p align="center">
  <a href="#-features">Features</a> • 
  <a href="#-user-guide">User Guide</a> • 
  <a href="#-installation--launch-for-developers">Installation</a> • 
  <a href="https://result-proxy.ru/">Project Website</a>
</p>

<p align="center">
  <a href="README.md">Русский</a> | <b>English</b>
</p>

---

## ✨ Features

- Built-in ad blocker powered by `@ghostery/adblocker`
- HTTP and SOCKS proxy support
- Modern user interface built with React and Tailwind CSS
- Cross-platform (Windows, macOS, Linux)
- Multi-language interface (integrated with `i18next`)

## 📖 User Guide

### After Installation
Upon launching the application, you will be greeted by the main page.
You cannot start a proxy from the main page right away. To enable them, you can purchase proxies from our [partner links](https://result-proxy.ru/#promo) with a 5% discount using the promo code **resultpoint**.
If you already have a proxy, you can skip this step.

<p align="center">
  <img src="https://result-proxy.ru/img/main-en.webp" width="600" alt="Main page">
</p>

### Purchasing Proxies
In the "Buy Proxy" tab, you can purchase proxies from one of our partners with a 5% discount.

<p align="center">
  <img src="https://result-proxy.ru/img/buy-en.webp" width="600" alt="Buy proxies">
</p>

### Adding Configuration
By clicking on the "Add server" field on the main page or navigating to the "Add" tab, you can enter your proxy details.
Authorization for the proxy is optional, but if the proxy requires a login and password, you will need to enter them.
You can also add one or multiple proxies by pasting them from the clipboard or from .txt/.csv files.

<p align="center">
  <img src="https://result-proxy.ru/img/add-en.webp" width="600" alt="Adding configuration">
</p>

### Profile List
After adding a proxy, you will be taken to the "Proxy List" page. Here you can connect to available proxies, delete or edit them, and see their ping.
Clicking on the card or the connect button will establish a connection to your server.
Proxies can also be started and stopped directly from the main page.

<p align="center">
  <img src="https://result-proxy.ru/img/list-en.webp" width="600" alt="Profile list">
</p>

### Active Proxy
On the main page, you can now see your active connection. Clicking on the card will reveal a list of other proxies.
Here you can edit the proxy and view internet usage in the "Downloaded" and "Uploaded" panels.
To disconnect the proxy, simply click the green toggle button.

<p align="center">
  <img src="https://result-proxy.ru/img/start-en.webp" width="600" alt="Active proxy">
</p>

### Smart Rules
On the "Smart Rules" page, you can configure which services will be proxied.
You can choose from two modes: Global and Smart (routes only blocked resources; the list may not contain resources unavailable in your specific country).
In the "Exclusion Sites" tab, you can specify individual sites or domains that will not be proxied (Example of adding a whole domain: `*.com`).

<p align="center">
  <img src="https://result-proxy.ru/img/rules-1-en.webp" width="600" alt="Smart rules — sites">
</p>

In the "Exclusion Apps" tab, you can select application executable files that will not be proxied using the OS file explorer or by entering the program name manually.

<p align="center">
  <img src="https://result-proxy.ru/img/rules-2-en.webp" width="600" alt="Smart rules — apps">
</p>

### Logs Page
On this page, you can view the status of your proxy and see which services are currently being proxied or bypassed.

<p align="center">
  <img src="https://result-proxy.ru/img/logs-en.webp" width="600" alt="Logs page">
</p>

### Application Settings
In the settings, you can enable application autostart and the Kill Switch function, which will instantly disconnect your internet connection if the proxy goes down.
You can also export and import your application configuration by creating an encryption password to prevent your data from being stolen.
An ad blocker is also available in the settings (note: it does not support inline blocking, meaning banners in YouTube search may still appear, but ads inside videos will be blocked).

<p align="center">
  <img src="https://result-proxy.ru/img/settings-en.webp" width="600" alt="Application settings">
</p>

## 🚀 Installation & Launch (for developers)

### Prerequisites

- Node.js (LTS version recommended)
- npm

### Steps

1. Clone the repository and navigate to the project directory:
   ```bash
   git clone <your_repository_link>
   cd ResultProxy
   ```

2. Install dependencies:
   ```bash
   npm install --legacy-peer-deps
   ```

3. Run the project in development mode:
   ```bash
   npm run dev
   ```
   *This command will simultaneously start the Vite process for React and the main Electron application window.*

## 📦 Building the Application

To build the installer files like `.exe`, `.AppImage`, and other formats, use the following commands:

- **For Windows:**
  ```bash
  npm run package
  ```

- **For Linux:**
  ```bash
  npm run package:linux
  ```

## 🛠 Tech Stack

- **Cross-platform framework:** [Electron](https://www.electronjs.org/)
- **Frontend:** [React](https://reactjs.org/), [Vite](https://vitejs.dev/), [Tailwind CSS](https://tailwindcss.com/)
- **Proxy and Networking:** `proxy-chain`, `socks`, `express`
- **Ad Blocking:** [@ghostery/adblocker](https://github.com/ghostery/adblocker)

---

**More information and app download:** [https://result-proxy.ru/](https://result-proxy.ru/)
