import Foundation

let complete = DispatchSemaphore(value: 0)
var success = false
let session = URLSession(configuration: .ephemeral)
var request = URLRequest(url: URL(string: CommandLine.arguments[1])!)
request.timeoutInterval = 6
session.dataTask(with: request) { data, response, error in
  success = error == nil && (response as? HTTPURLResponse)?.statusCode == 200 &&
    data == Data("shadowbox-real-core-ok".utf8)
  complete.signal()
}.resume()
_ = complete.wait(timeout: .now() + 8)
exit(success ? 0 : 1)
