$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing
$code = @"
using System;
using System.Drawing;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;

public class WinCap {
    [DllImport("user32.dll")]
    public static extern bool GetWindowRect(IntPtr hWnd, out RECT lpRect);
    [DllImport("user32.dll")]
    public static extern bool GetClientRect(IntPtr hWnd, out RECT lpRect);
    [DllImport("user32.dll")]
    public static extern bool ClientToScreen(IntPtr hWnd, ref POINT lpPoint);
    [DllImport("user32.dll")]
    public static extern bool PrintWindow(IntPtr hWnd, IntPtr hdcBlt, uint nFlags);
    [DllImport("user32.dll")]
    public static extern bool SetForegroundWindow(IntPtr hWnd);
    [DllImport("user32.dll")]
    public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
    [DllImport("user32.dll")]
    public static extern bool SetCursorPos(int X, int Y);
    [DllImport("user32.dll")]
    public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
    public const uint PW_RENDERFULLCONTENT = 2;
    public const int SW_RESTORE = 9;
    public const uint MOUSEEVENTF_LEFTDOWN = 0x0002;
    public const uint MOUSEEVENTF_LEFTUP = 0x0004;
    [StructLayout(LayoutKind.Sequential)]
    public struct RECT { public int Left; public int Top; public int Right; public int Bottom; }
    [StructLayout(LayoutKind.Sequential)]
    public struct POINT { public int X; public int Y; }

    public static void SaveWindowPng(IntPtr hwnd, string path) {
        RECT r;
        if (!GetWindowRect(hwnd, out r)) throw new Exception("GetWindowRect failed");
        int w = r.Right - r.Left;
        int h = r.Bottom - r.Top;
        if (w < 10 || h < 10) throw new Exception("bad size " + w + "x" + h);
        using (Bitmap bmp = new Bitmap(w, h)) {
            using (Graphics g = Graphics.FromImage(bmp)) {
                IntPtr hdc = g.GetHdc();
                try {
                    if (!PrintWindow(hwnd, hdc, PW_RENDERFULLCONTENT)) {
                        if (!PrintWindow(hwnd, hdc, 0))
                            throw new Exception("PrintWindow failed");
                    }
                } finally {
                    g.ReleaseHdc(hdc);
                }
            }
            bmp.Save(path, ImageFormat.Png);
        }
    }

    public static void ClickClient(IntPtr hwnd, int cx, int cy) {
        var pt = new POINT { X = cx, Y = cy };
        if (!ClientToScreen(hwnd, ref pt)) throw new Exception("ClientToScreen failed");
        SetCursorPos(pt.X, pt.Y);
        mouse_event(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, UIntPtr.Zero);
        mouse_event(MOUSEEVENTF_LEFTUP, 0, 0, 0, UIntPtr.Zero);
    }
}
"@
Add-Type -TypeDefinition $code -ReferencedAssemblies System.Drawing

$exe = Join-Path $PSScriptRoot "..\build\bin\ResultProxy.exe" | Resolve-Path
$outDir = Join-Path $PSScriptRoot "..\docs\images\readme" | Resolve-Path

$p = Start-Process -FilePath $exe -PassThru -WindowStyle Normal
$deadline = (Get-Date).AddSeconds(45)
$hwnd = [IntPtr]::Zero
while ((Get-Date) -lt $deadline) {
    Start-Sleep -Milliseconds 400
    $p.Refresh()
    if ($p.HasExited) { throw "Process exited early" }
    $hwnd = $p.MainWindowHandle
    if ($hwnd -ne [IntPtr]::Zero) { break }
}
if ($hwnd -eq [IntPtr]::Zero) {
    Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
    throw "MainWindowHandle not available"
}
[void][WinCap]::ShowWindow($hwnd, [WinCap]::SW_RESTORE)
[void][WinCap]::SetForegroundWindow($hwnd)
Start-Sleep -Seconds 3

function Shot([string]$name) {
    $path = Join-Path $outDir $name
    [WinCap]::SaveWindowPng($hwnd, $path)
    Write-Host $path
}

$cx = 130
$nav = @(104, 156, 208, 260, 312, 364)
$shots = @(
    @{ f = "01-home.png"; i = 0 },
    @{ f = "02-add.png"; i = 2 },
    @{ f = "03-list.png"; i = 3 },
    @{ f = "04-rules.png"; i = 4 },
    @{ f = "05-logs.png"; i = 5 },
    @{ f = "06-settings.png"; i = 6 }
)

foreach ($s in $shots) {
    [void][WinCap]::SetForegroundWindow($hwnd)
    Start-Sleep -Milliseconds 300
    [WinCap]::ClickClient($hwnd, $cx, $nav[$s.i])
    Start-Sleep -Seconds 2
    Shot $s.f
}

Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
Write-Host "Done."
