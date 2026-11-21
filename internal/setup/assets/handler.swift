// rdxt's document handler: the app macOS opens .md and .dok files with
// (Cmd-click in Rex, double-click in Finder). It hands each file to
// `rdxt open --click`, which shows it in Rex where `rdxt setup --open` said
// (a floating window, or a pane beside the focused one), and quits.
// Built and installed by `rdxt setup`.
import AppKit
import CoreServices

let rdxt = "__RDXT__"
let rexBin = "__REX__"
let bundleID = "com.travelingflwr.rdxt"
let types = ["net.daringfireball.markdown", "com.travelingflwr.dok"]

func setDefault() {
    var previous: [String: String] = [:]
    for t in types {
        if let h = LSCopyDefaultRoleHandlerForContentType(t as CFString, .all)?.takeRetainedValue() as String?,
           h.lowercased() != bundleID {
            previous[t] = h
        }
        LSSetDefaultRoleHandlerForContentType(t as CFString, .all, bundleID as CFString)
    }
    let data = try! JSONSerialization.data(withJSONObject: previous)
    print(String(data: data, encoding: .utf8)!)
}

func restore(_ pairs: ArraySlice<String>) {
    for pair in pairs {
        let parts = pair.split(separator: "=", maxSplits: 1).map(String.init)
        if parts.count == 2 {
            LSSetDefaultRoleHandlerForContentType(parts[0] as CFString, .all, parts[1] as CFString)
        }
    }
}

func open(_ path: String) {
    let p = Process()
    p.executableURL = URL(fileURLWithPath: rdxt)
    p.arguments = ["open", "--click", path]
    var env = ProcessInfo.processInfo.environment
    env["RDXT_REX"] = rexBin
    env["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:" + (env["PATH"] ?? "")
    p.environment = env
    do {
        try p.run()
        p.waitUntilExit()
        if p.terminationStatus == 0 { return }
    } catch {}
    // Rex isn't running: start it, then try once more.
    if let rexApp = NSWorkspace.shared.urlForApplication(withBundleIdentifier: "com.superlogical.rex") {
        NSWorkspace.shared.open(rexApp)
    }
    Thread.sleep(forTimeInterval: 3)
    if let q = try? Process.run(URL(fileURLWithPath: rdxt), arguments: ["open", "--click", path]) {
        q.waitUntilExit()
    }
}

let args = CommandLine.arguments
if args.count > 1 && args[1] == "--set-default" {
    setDefault()
    exit(0)
}
if args.count > 1 && args[1] == "--restore" {
    restore(args.dropFirst(2))
    exit(0)
}

class Delegate: NSObject, NSApplicationDelegate {
    func application(_ app: NSApplication, open urls: [URL]) {
        for u in urls where u.isFileURL { open(u.path) }
        NSApp.terminate(nil)
    }
    func applicationDidFinishLaunching(_ n: Notification) {
        // Launched without a file (from Finder or Spotlight): nothing to do.
        DispatchQueue.main.asyncAfter(deadline: .now() + 5) { NSApp.terminate(nil) }
    }
}

let app = NSApplication.shared
let delegate = Delegate()
app.delegate = delegate
app.setActivationPolicy(.prohibited)
app.run()
