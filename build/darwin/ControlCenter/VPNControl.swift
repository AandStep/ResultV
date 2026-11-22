import AppIntents
import AppKit
import Darwin
import SwiftUI
import WidgetKit

@main
struct VPNControl: ControlWidget {
    static let kind = "com.wails.ResultV.vpn"

    var body: some ControlWidgetConfiguration {
        StaticControlConfiguration(kind: Self.kind, provider: VPNValueProvider()) { connected in
            ControlWidgetToggle("ResultV", isOn: connected, action: SetVPN()) { connected in
                Label(connected ? "On" : "Off", systemImage: "shield.lefthalf.filled")
            }
        }
        .displayName("ResultV")
        .description("Connect or disconnect ResultV.")
    }
}

struct VPNValueProvider: ControlValueProvider {
    let previewValue = false

    func currentValue() async throws -> Bool {
        try ControlConnection.request(.status) == true
    }
}

struct SetVPN: SetValueIntent {
    static let title: LocalizedStringResource = "Set ResultV connection"

    @Parameter(title: "Connected")
    var value: Bool

    @MainActor
    func perform() async throws -> some IntentResult {
        let command: ControlCommand = value ? .connect : .disconnect
        if try ControlConnection.request(command) == nil, value {
            let appURL = Bundle.main.bundleURL
                .deletingLastPathComponent()
                .deletingLastPathComponent()
                .deletingLastPathComponent()
            let configuration = NSWorkspace.OpenConfiguration()
            configuration.activates = false
            configuration.arguments = ["--tray"]
            _ = try await NSWorkspace.shared.openApplication(at: appURL, configuration: configuration)
            // The listener is installed during startup, after the app loads its settings.
            for _ in 0..<40 {
                try await Task.sleep(for: .milliseconds(250))
                if try ControlConnection.request(command) != nil {
                    return .result()
                }
            }
            throw ControlError.unavailable
        }
        return .result()
    }
}

enum ControlError: Error, CustomLocalizedStringResourceConvertible {
    case unavailable, rejected

    var localizedStringResource: LocalizedStringResource {
        switch self {
        case .unavailable: "Unable to reach ResultV. Open the app and try again."
        case .rejected: "Select a server in ResultV, or check the app log."
        }
    }
}

enum ControlCommand: UInt8 {
    case status = 115
    case connect = 49
    case disconnect = 48
}

enum ControlConnection {
    // A nil reply means the app has not created a listener.
    static func request(_ command: ControlCommand) throws -> Bool? {
        guard let group = Bundle.main.object(forInfoDictionaryKey: "ResultVControlGroup") as? String,
              let folder = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: group)
        else { throw ControlError.unavailable }
        let path = Array(folder.appendingPathComponent("ctl.sock").path.utf8CString)
        var address = sockaddr_un()
        guard path.count <= MemoryLayout.size(ofValue: address.sun_path)
        else { throw ControlError.unavailable }
        address.sun_family = sa_family_t(AF_UNIX)
        address.sun_len = UInt8(MemoryLayout<sockaddr_un>.size)
        withUnsafeMutableBytes(of: &address.sun_path) { buffer in
            path.withUnsafeBytes { buffer.copyBytes(from: $0) }
        }
        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else { throw ControlError.unavailable }
        defer { close(fd) }
        var noSignal: Int32 = 1
        var timeout = timeval(tv_sec: 1, tv_usec: 0)
        guard setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &noSignal, socklen_t(MemoryLayout<Int32>.size)) == 0,
              setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, socklen_t(MemoryLayout<timeval>.size)) == 0,
              setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &timeout, socklen_t(MemoryLayout<timeval>.size)) == 0
        else { throw ControlError.unavailable }
        let result = withUnsafePointer(to: &address) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                connect(fd, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        if result != 0 {
            if errno == ENOENT || errno == ECONNREFUSED { return nil }
            throw ControlError.unavailable
        }
        // The GUI runs under the user or through the existing root relaunch.
        var uid: uid_t = 0
        var gid: gid_t = 0
        guard getpeereid(fd, &uid, &gid) == 0, uid == getuid() || uid == 0
        else { throw ControlError.unavailable }

        var input = command.rawValue
        var reply: UInt8 = 0
        guard write(fd, &input, 1) == 1, read(fd, &reply, 1) == 1
        else { throw ControlError.unavailable }
        guard reply == 48 || reply == 49 else { throw ControlError.rejected }
        return reply == ControlCommand.connect.rawValue
    }
}
