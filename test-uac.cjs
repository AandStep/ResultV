const { spawn } = require("child_process");

const exePath = process.execPath;
const args = ["-v"];

console.log(`Executable: ${exePath}`);

const psArgs = [
  "-WindowStyle",
  "Hidden",
  "-Command",
  `Start-Sleep -Seconds 2; Start-Process -FilePath '${exePath}' -ArgumentList '${args.join(" ")}' -Verb RunAs`,
];

console.log(`Command: powershell ${psArgs.join(" ")}`);

const child = spawn("powershell", psArgs, {
  detached: true,
  stdio: "ignore",
});

child.unref();
console.log("PS spawn success, exiting parent process...");
process.exit(0);
