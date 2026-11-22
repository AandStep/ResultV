import WidgetKit

@_cdecl("ResultVReloadVPNControl")
public func reloadVPNControl() {
    ControlCenter.shared.reloadControls(ofKind: "com.wails.ResultV.vpn")
}
