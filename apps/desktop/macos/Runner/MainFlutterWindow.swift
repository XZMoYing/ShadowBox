import Cocoa
import FlutterMacOS

class MainFlutterWindow: NSWindow {
  private var controlChannel: FlutterMethodChannel?
  override func awakeFromNib() {
    let controller = FlutterViewController()
    contentViewController = controller
    title = "ShadowBox"
    minSize = NSSize(width: 850, height: 620)
    setContentSize(NSSize(width: 1160, height: 780))
    center()
    RegisterGeneratedPlugins(registry: controller)
    controlChannel = FlutterMethodChannel(name: "shadowbox/window", binaryMessenger: controller.engine.binaryMessenger)
    controlChannel?.setMethodCallHandler { [weak self] call, result in
      if call.method == "show" {
        NSApp.setActivationPolicy(.regular)
        self?.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        result(nil)
      } else { result(FlutterMethodNotImplemented) }
    }
    super.awakeFromNib()
    NSApp.setActivationPolicy(.regular)
    makeKeyAndOrderFront(nil)
    NSApp.activate(ignoringOtherApps: true)
  }
}
