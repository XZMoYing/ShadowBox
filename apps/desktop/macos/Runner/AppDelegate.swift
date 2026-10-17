import Cocoa
import FlutterMacOS

@main
class AppDelegate: FlutterAppDelegate {
  private var closingLastWindow = false
  private var quitting = false
  override func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { closingLastWindow = true; return true }

  // Closing the last window unloads only Flutter. Explicit Quit (including
  // Command-Q) asks the tray to restore networking and stop the complete session.
  override func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
    if closingLastWindow { return .terminateNow }
    if quitting { return .terminateLater }
    quitting = true
    let path = ProcessInfo.processInfo.environment["SHADOWBOX_DATA_DIR"] ??
      FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Application Support/ShadowBox").path
    do {
      let data = try Data(contentsOf: URL(fileURLWithPath: path).appendingPathComponent("service.json"))
      let info = try JSONSerialization.jsonObject(with: data) as? [String: Any]
      guard let endpoint = info?["endpoint"] as? String, let token = info?["token"] as? String,
            let url = URL(string: endpoint + "/v1/quit") else { throw NSError(domain: "ShadowBox", code: 1) }
      var request = URLRequest(url: url, timeoutInterval: 5)
      request.httpMethod = "POST"
      request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
      request.httpBody = Data("{}".utf8)
      URLSession.shared.dataTask(with: request) { _, response, error in
        if error != nil || (response as? HTTPURLResponse)?.statusCode != 200 {
          self.cancelQuit()
        } else {
          // A directly opened helper is not a child tracked by the tray. Wait
          // for the service to finish cleanup before terminating that UI too.
          self.waitForShutdown(endpoint: endpoint, token: token, deadline: Date().addingTimeInterval(60))
        }
      }.resume()
    } catch { cancelQuit() }
    return .terminateLater
  }
  private func waitForShutdown(endpoint: String, token: String, deadline: Date) {
    guard Date() < deadline, let url = URL(string: endpoint + "/v1/status") else { cancelQuit(); return }
    var request = URLRequest(url: url, timeoutInterval: 3)
    request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
    URLSession.shared.dataTask(with: request) { data, response, error in
      if let networkError = error as? URLError,
         [.cannotConnectToHost, .networkConnectionLost].contains(networkError.code) {
        DispatchQueue.main.async { NSApp.reply(toApplicationShouldTerminate: true) }
        return
      }
      guard error == nil, (response as? HTTPURLResponse)?.statusCode == 200,
            let data = data,
            let state = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
            state["quit_requested"] as? Bool == true else { self.cancelQuit(); return }
      DispatchQueue.global().asyncAfter(deadline: .now() + 0.3) {
        self.waitForShutdown(endpoint: endpoint, token: token, deadline: deadline)
      }
    }.resume()
  }
  private func cancelQuit() {
    DispatchQueue.main.async {
      self.quitting = false
      NSApp.reply(toApplicationShouldTerminate: false)
      self.showQuitError()
    }
  }
  private func showQuitError() {
    let alert = NSAlert()
    alert.messageText = "无法完成退出"
    alert.informativeText = "后台服务未响应，请从托盘重试退出并恢复网络。"
    alert.runModal()
  }
  override func applicationSupportsSecureRestorableState(_ app: NSApplication) -> Bool { true }
}
